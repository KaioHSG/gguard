package engine

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/KaioHSG/gguard/ggs"
	"github.com/KaioHSG/gguard/notify"
)

type Dispatcher struct {
	name string
	vars map[string]string
}

func NewDispatcher(name string, vars map[string]string) *Dispatcher {
	return &Dispatcher{name: name, vars: vars}
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
	case "notify":
		return d.execNotify(args)
	case "run":
		return d.execRun(args)
	case "delete":
		return d.execDelete(args)
	case "webhook":
		return d.execWebhook(args)
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
	// A message is also surfaced as a desktop notification so the user gets
	// feedback even when the tray is not visible.
	d.notify(args[0])
	return nil
}

func (d *Dispatcher) execNotify(args []string) error {
	if len(args) == 0 {
		return nil
	}
	d.notify(args[0])
	return nil
}

func (d *Dispatcher) notify(message string) {
	title := d.name
	if title == "" {
		title = "GopherGuard"
	}
	if err := notify.Notify(title, message); err != nil {
		log.Printf("[notify] %v", err)
	}
}

// execRun runs an arbitrary shell command. It is gated behind
// trusted:true because it can do anything the user can.
func (d *Dispatcher) execRun(args []string) error {
	if d.vars["trusted"] != "true" {
		return fmt.Errorf("run: requires trusted:true in guards.json")
	}
	if len(args) == 0 {
		return fmt.Errorf("run requires a command")
	}

	line := strings.Join(args, " ")
	cmd := shellCommand(line)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	log.Printf("[run] %s", line)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("run %q: %w", line, err)
	}
	return nil
}

// execDelete removes files or directories. Gated behind trusted:true.
func (d *Dispatcher) execDelete(args []string) error {
	if d.vars["trusted"] != "true" {
		return fmt.Errorf("delete: requires trusted:true in guards.json")
	}
	if len(args) == 0 {
		return fmt.Errorf("delete requires a path")
	}

	for _, p := range args {
		if err := os.RemoveAll(p); err != nil {
			return fmt.Errorf("delete %s: %w", p, err)
		}
		log.Printf("[delete] removed %s", p)
	}
	return nil
}

// execWebhook POSTs a body to a URL, useful to ping a server after a backup.
func (d *Dispatcher) execWebhook(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("webhook requires a URL")
	}

	url := args[0]
	body := ""
	if len(args) > 1 {
		body = strings.Join(args[1:], " ")
	}

	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		return fmt.Errorf("webhook: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("webhook %s: %w", url, err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook %s: server returned %s", url, resp.Status)
	}
	log.Printf("[webhook] %s -> %s", url, resp.Status)
	return nil
}

// ignoreFileName is read from the root of the source when it exists, so a
// routine can keep its exclusions next to what it backups.
const ignoreFileName = ".ggignore"

func (d *Dispatcher) execSync(args []string) error {
	deleteOrphans := false
	bidirectional := false
	var excludes []string
	var cleanArgs []string

	for i := 0; i < len(args); i++ {
		switch a := args[i]; {
		case a == "--delete":
			deleteOrphans = true
		case a == "--bidir":
			bidirectional = true
		case a == "--exclude":
			if i+1 >= len(args) {
				return fmt.Errorf("--exclude requires a pattern")
			}
			i++
			excludes = append(excludes, args[i])
		case strings.HasPrefix(a, "--exclude="):
			excludes = append(excludes, strings.TrimPrefix(a, "--exclude="))
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

	ignore := NewIgnore()
	for _, pattern := range excludes {
		ignore.Add(pattern)
	}
	if err := ignore.AddFile(filepath.Join(src, ignoreFileName)); err != nil {
		log.Printf("[sync] cannot read %s: %v", ignoreFileName, err)
	}
	if !ignore.Empty() {
		log.Printf("[sync] leaving behind: %s", strings.Join(ignore.Patterns(), ", "))
	}

	if err := syncDir(src, dest, ignore); err != nil {
		return err
	}

	if bidirectional {
		if err := syncDir(dest, src, ignore); err != nil {
			return err
		}
	}

	if deleteOrphans {
		removeOrphans(dest, src, ignore)
		if bidirectional {
			removeOrphans(src, dest, ignore)
		}
	}

	return nil
}

func syncDir(src, dest string, ignore *Ignore) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return fmt.Errorf("sync: walk error at %s: %w", path, err)
		}

		relPath, _ := filepath.Rel(src, path)
		if relPath == "." {
			return nil
		}

		// Skip whole trees, not just the file that matched.
		if ignore.Match(relPath, info.IsDir()) {
			if info.IsDir() {
				return filepath.SkipDir
			}
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

func removeOrphans(dest, src string, ignore *Ignore) {
	filepath.Walk(dest, func(destPath string, destInfo os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		relPath, _ := filepath.Rel(dest, destPath)
		if relPath == "." {
			return nil
		}

		// What the sync leaves behind is not an orphan: it was never copied.
		if ignore.Match(relPath, destInfo.IsDir()) {
			if destInfo.IsDir() {
				return filepath.SkipDir
			}
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
