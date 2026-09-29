package engine

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/KaioHSG/gguard/ggs"
)

type Dispatcher struct {
	vars map[string]string
}

func NewDispatcher(vars map[string]string) *Dispatcher {
	return &Dispatcher{vars: vars}
}

func (d *Dispatcher) Execute(cmd ggs.Command) error {
	args := make([]string, len(cmd.Args))
	for i, arg := range cmd.Args {
		args[i] = d.resolve(arg)
	}

	switch cmd.Verb {
	case "zip":
		return d.execZip(args)
	case "sync":
		return d.execSync(args)
	case "message":
		return d.execMessage(args)
	default:
		return fmt.Errorf("unknown command: %s", cmd.Verb)
	}
}

func (d *Dispatcher) resolve(s string) string {
	re := regexp.MustCompile(`\{(\w+)\}`)
	for {
		prev := s
		s = re.ReplaceAllStringFunc(s, func(match string) string {
			key := match[1 : len(match)-1]
			if val, ok := d.vars[key]; ok {
				return val
			}
			return match
		})
		if s == prev {
			break
		}
	}
	return s
}

func (d *Dispatcher) execZip(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("zip requires source and destination")
	}
	dest := args[0]
	src := args[1]

	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return fmt.Errorf("zip: cannot create dest dir: %w", err)
	}

	f, err := os.Create(dest)
	if err != nil {
		return fmt.Errorf("zip: cannot create file %s: %w", dest, err)
	}
	defer f.Close()

	zw := zip.NewWriter(f)
	defer zw.Close()

	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return fmt.Errorf("zip: walk error at %s: %w", path, err)
		}

		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return fmt.Errorf("zip: header error for %s: %w", path, err)
		}

		relPath, _ := filepath.Rel(src, path)
		if relPath == "." {
			return nil
		}
		relPath = strings.ReplaceAll(relPath, "\\", "/")
		header.Name = relPath
		header.Method = zip.Deflate

		if info.IsDir() {
			header.Name += "/"
			_, err = zw.CreateHeader(header)
			return err
		}

		w, err := zw.CreateHeader(header)
		if err != nil {
			return fmt.Errorf("zip: create header error: %w", err)
		}

		file, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("zip: cannot open %s: %w", path, err)
		}
		defer file.Close()

		_, err = io.Copy(w, file)
		return err
	})
}

func (d *Dispatcher) execMessage(args []string) error {
	if len(args) == 0 {
		return nil
	}
	fmt.Println(args[0])
	return nil
}

func (d *Dispatcher) execSync(args []string) error {
	deleteOrphans := false
	bidirectional := false
	var cleanArgs []string
	for _, a := range args {
		switch a {
		case "--delete":
			deleteOrphans = true
		case "--bidir":
			bidirectional = true
		default:
			cleanArgs = append(cleanArgs, a)
		}
	}

	if deleteOrphans && d.vars["trusted"] != "true" {
		deleteOrphans = false
		log.Printf("[sync] --delete ignored: trusted:true required in guards.json")
	}
	if len(cleanArgs) < 2 {
		return fmt.Errorf("sync requires destination and source")
	}
	dest := cleanArgs[0]
	src := cleanArgs[1]

	if err := syncDir(src, dest); err != nil {
		return err
	}

	if bidirectional {
		if err := syncDir(dest, src); err != nil {
			return err
		}
	}

	if deleteOrphans {
		removeOrphans(dest, src)
		if bidirectional {
			removeOrphans(src, dest)
		}
	}

	return nil
}

func syncDir(src, dest string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return fmt.Errorf("sync: walk error at %s: %w", path, err)
		}

		relPath, _ := filepath.Rel(src, path)
		if relPath == "." {
			return nil
		}

		destPath := filepath.Join(dest, relPath)

		if info.IsDir() {
			return os.MkdirAll(destPath, info.Mode())
		}

		if sameFile(path, destPath) {
			return nil
		}

		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return fmt.Errorf("sync: cannot create dir %s: %w", filepath.Dir(destPath), err)
		}

		srcFile, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("sync: cannot open %s: %w", path, err)
		}
		defer srcFile.Close()

		dstFile, err := os.Create(destPath)
		if err != nil {
			return fmt.Errorf("sync: cannot create %s: %w", destPath, err)
		}

		if _, err := io.Copy(dstFile, srcFile); err != nil {
			dstFile.Close()
			return fmt.Errorf("sync: copy error: %w", err)
		}
		if err := dstFile.Close(); err != nil {
			return fmt.Errorf("sync: close error: %w", err)
		}

		os.Chtimes(destPath, time.Now(), info.ModTime())

		return nil
	})
}

func sameFile(a, b string) bool {
	ai, errA := os.Stat(a)
	bi, errB := os.Stat(b)
	if errA != nil || errB != nil {
		return false
	}
	if ai.Size() != bi.Size() {
		return false
	}

	af, err := os.Open(a)
	if err != nil {
		return false
	}
	defer af.Close()

	bf, err := os.Open(b)
	if err != nil {
		return false
	}
	defer bf.Close()

	bufA := make([]byte, 64*1024)
	bufB := make([]byte, 64*1024)
	for {
		na, errA := af.Read(bufA)
		nb, errB := bf.Read(bufB)
		if na != nb {
			return false
		}
		if !bytes.Equal(bufA[:na], bufB[:nb]) {
			return false
		}
		if errA == io.EOF && errB == io.EOF {
			return true
		}
		if errA != nil || errB != nil {
			return false
		}
	}
}

func removeOrphans(dest, src string) {
	filepath.Walk(dest, func(destPath string, destInfo os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		relPath, _ := filepath.Rel(dest, destPath)
		if relPath == "." {
			return nil
		}

		srcPath := filepath.Join(src, relPath)
		if _, err := os.Stat(srcPath); os.IsNotExist(err) {
			if err := os.RemoveAll(destPath); err != nil {
				log.Printf("[sync --delete] cannot remove orphan %s: %v", destPath, err)
			} else {
				log.Printf("[sync --delete] removed orphan: %s", destPath)
			}
		}
		return nil
	})
}
