//go:build ignore

// build.go centralizes the build for every platform, replacing the
// platform-specific build.ps1 and build.sh scripts.
//
// It is a normal Go program excluded from the package via the "ignore"
// build tag, so it can simply be run:
//
//	go run build.go          # dev build for the current OS
//	go run build.go --dist   # cross-platform release artifacts in dist/
package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const ldflags = "-s -w"

// pkgDir is the main package directory, relative to the module root. The
// resources/ and winres/ assets live next to it because go:embed cannot reach
// outside the package directory.
const pkgDir = "cmd/gguard"

type target struct {
	goos    string
	goarch  string
	archive string
}

// Release artifacts. Names must match the files referenced by the release
// workflow in .github/workflows/build.yaml.
var distTargets = []target{
	{"windows", "amd64", "GopherGuard-Windows-x64.zip"},
	{"windows", "arm64", "GopherGuard-Windows-ARM.zip"},
	{"linux", "amd64", "GopherGuard-Linux-x64.tar.gz"},
	{"linux", "arm64", "GopherGuard-Linux-ARM.tar.gz"},
}

func main() {
	dist := false
	for _, arg := range os.Args[1:] {
		switch arg {
		case "--dist", "dist":
			dist = true
		case "-h", "--help", "help":
			usage()
			return
		default:
			fmt.Fprintf(os.Stderr, "build: unknown argument %q\n", arg)
			usage()
			os.Exit(2)
		}
	}

	root := moduleRoot()

	if dist {
		if err := runWinres(root, "amd64,arm64"); err != nil {
			fatalf("go-winres: %v", err)
		}
		if err := os.MkdirAll(filepath.Join(root, "dist"), 0o755); err != nil {
			fatalf("cannot create dist: %v", err)
		}
		for _, t := range distTargets {
			if err := buildDist(root, t); err != nil {
				fatalf("%s/%s: %v", t.goos, t.goarch, err)
			}
		}
	} else {
		// go-winres only stamps Windows resources; on other platforms (or when
		// it is not installed) a failure is not fatal.
		if err := runWinres(root, ""); err != nil {
			fmt.Fprintf(os.Stderr, "build: warning: skipping go-winres: %v\n", err)
		}
		if err := buildDev(root); err != nil {
			fatalf("%v", err)
		}
	}

	fmt.Println("Finished.")
}

func usage() {
	fmt.Fprint(os.Stderr, `usage: go run build.go [--dist]

  (no flag)  dev build for the current OS
  --dist     cross-platform release artifacts in dist/
`)
}

// moduleRoot returns the directory containing build.go, so the script works
// regardless of the current working directory.
func moduleRoot() string {
	if _, file, _, ok := runtime.Caller(0); ok {
		return filepath.Dir(file)
	}
	dir, _ := os.Getwd()
	return dir
}

func buildDev(root string) error {
	out := "gguard"
	if runtime.GOOS == "windows" {
		out = "gguard.exe"
	}
	fmt.Printf("Building %s...\n", out)
	return goBuild(root, runtime.GOOS, runtime.GOARCH, out)
}

func buildDist(root string, t target) error {
	fmt.Printf("Building %s-%s...\n", t.goos, t.goarch)

	bin := "gguard"
	if t.goos == "windows" {
		bin = "gguard.exe"
	}
	binPath := filepath.Join(root, "dist", bin)

	if err := goBuild(root, t.goos, t.goarch, binPath); err != nil {
		return err
	}
	defer os.Remove(binPath)

	archivePath := filepath.Join(root, "dist", t.archive)
	if strings.HasSuffix(t.archive, ".zip") {
		return createZip(archivePath, binPath, bin)
	}
	return createTarGz(archivePath, binPath, bin)
}

func goBuild(root, goos, goarch, out string) error {
	cmd := exec.Command("go", "build", "-ldflags", ldflags, "-o", out, "./"+pkgDir)
	cmd.Dir = root
	cmd.Env = envWith(map[string]string{"GOOS": goos, "GOARCH": goarch})
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func runWinres(root, arch string) error {
	if _, err := exec.LookPath("go-winres"); err != nil {
		return err
	}
	args := []string{"make"}
	if arch != "" {
		args = append(args, "--arch", arch)
	}
	// go-winres reads winres/winres.json and writes rsrc_windows_*.syso next to
	// the main package, so it must run from the package directory.
	cmd := exec.Command("go-winres", args...)
	cmd.Dir = filepath.Join(root, pkgDir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func createZip(archivePath, srcPath, name string) error {
	src, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer src.Close()

	f, err := os.Create(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()

	zw := zip.NewWriter(f)
	w, err := zw.Create(name)
	if err != nil {
		return err
	}
	if _, err := io.Copy(w, src); err != nil {
		return err
	}
	return zw.Close()
}

func createTarGz(archivePath, srcPath, name string) error {
	info, err := os.Stat(srcPath)
	if err != nil {
		return err
	}

	f, err := os.Create(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()

	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)

	hdr := &tar.Header{
		Name:    name,
		Mode:    0o755,
		Size:    info.Size(),
		ModTime: info.ModTime(),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}

	src, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer src.Close()

	if _, err := io.Copy(tw, src); err != nil {
		return err
	}
	if err := tw.Close(); err != nil {
		return err
	}
	return gz.Close()
}

// envWith returns the current environment with the given keys replaced,
// avoiding duplicate entries that would otherwise leak in on some platforms.
func envWith(extra map[string]string) []string {
	drop := make(map[string]bool, len(extra))
	for k := range extra {
		drop[strings.ToUpper(k)] = true
	}

	out := make([]string, 0, len(os.Environ())+len(extra))
	for _, kv := range os.Environ() {
		key := kv
		if i := strings.IndexByte(kv, '='); i >= 0 {
			key = kv[:i]
		}
		if !drop[strings.ToUpper(key)] {
			out = append(out, kv)
		}
	}
	for k, v := range extra {
		out = append(out, k+"="+v)
	}
	return out
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "build: "+format+"\n", args...)
	os.Exit(1)
}
