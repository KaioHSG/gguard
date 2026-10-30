//go:build ignore

// build.go centralizes the build for every platform, replacing the
// platform-specific build.ps1 and build.sh scripts.
//
// Every target produces three pieces:
//
//	gguard            the app: guards, service, dashboard, normal console CLI
//	gguard-launcher   Windows only: a windowless launcher (-H windowsgui) that
//	                  starts gguard for the GUI entry points (logon, shortcut)
//	gguard-setup      the installer, which carries the two binaries above
//
// It is a normal Go program excluded from the package via the "ignore"
// build tag, so it can simply be run:
//
//	go run build.go          # dev build for the current OS
//	go run build.go --dist   # the installer, named per platform, in dist/
package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

const ldflags = "-s -w"

// launcherFlags build the launcher without a console. That is the whole point
// of it: the logon entry, the Start Menu shortcut and every other GUI entry
// point start gguard through it, and a binary without a console never makes
// Windows flash one on screen.
const launcherFlags = "-H=windowsgui"

// launcherPkgDir and launcherName are the launcher package and its binary.
const (
	launcherPkgDir = "cmd/gguard-launcher"
	launcherName   = "gguard-launcher.exe"
)

// resourcePkgs get the Windows icon set, version info and manifest. The three
// packages keep their own winres/ directory (go-winres reads it from the
// package) but share the icons, which live next to the app.
var resourcePkgs = []string{pkgDir, launcherPkgDir, setupPkgDir}

// versionFile is where the release version lives.
const versionFile = "install/version.go"

// pkgDir is the app package directory, relative to the module root. The
// resources/ and winres/ assets live next to it because go:embed cannot reach
// outside the package directory.
const pkgDir = "cmd/gguard"

// setupPkgDir holds the installer, and payloadDir the binaries it embeds.
const (
	setupPkgDir = "cmd/gguard-setup"
	payloadDir  = "cmd/gguard-setup/payload"
	payloadKeep = "placeholder.txt"
)

type target struct {
	goos   string
	goarch string
	// asset is the release file name: the installer itself, named after the
	// platform it installs.
	asset string
}

// Release artifacts. Names must match the files referenced by the release
// workflow in .github/workflows/build.yaml.
var distTargets = []target{
	{"windows", "amd64", "GopherGuard-Windows-x64.exe"},
	{"windows", "arm64", "GopherGuard-Windows-ARM.exe"},
	{"linux", "amd64", "GopherGuard-Linux-x64"},
	{"linux", "arm64", "GopherGuard-Linux-ARM"},
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
		if err := stampResources(root, "amd64,arm64"); err != nil {
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
		// dev build for the machine we are on
		if err := stampResources(root, ""); err != nil {
			// go-winres only stamps Windows resources; on other platforms (or
			// when it is not installed) a failure is not fatal.
			fmt.Fprintf(os.Stderr, "build: warning: skipping go-winres: %v\n", err)
		}
		files, err := buildSet(root, runtime.GOOS, runtime.GOARCH, root)
		if err != nil {
			fatalf("%v", err)
		}
		for _, f := range files {
			fmt.Printf("  %s\n", filepath.Base(f))
		}
	}

	fmt.Println("Finished.")
}

func usage() {
	fmt.Fprint(os.Stderr, `usage: go run build.go [--dist]

  (no flag)  dev build for the current OS
  --dist     the installer, named per platform, in dist/
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

// buildSet builds the binaries of one target and the installer carrying them,
// writing them to outDir. It returns every file that belongs in the release.
func buildSet(root, goos, goarch, outDir string) ([]string, error) {
	// The payload stays behind on purpose: it is what go:embed picks up, so a
	// plain 'go build' or 'go test' keeps working after a build.go run.
	payload := filepath.Join(root, payloadDir)
	if err := resetPayload(payload); err != nil {
		return nil, err
	}

	appName := binaryName("gguard", goos)
	appPath := filepath.Join(payload, appName)
	if err := goBuild(root, goos, goarch, "./"+pkgDir, appPath, ldflags); err != nil {
		return nil, err
	}

	// The launcher only makes sense on Windows, where a console program started
	// by Explorer flashes a window. Elsewhere the app is started directly.
	if goos == "windows" {
		launcher := filepath.Join(payload, launcherName)
		if err := goBuild(root, goos, goarch, "./"+launcherPkgDir, launcher, ldflags+" "+launcherFlags); err != nil {
			return nil, err
		}
	}

	setupPath := filepath.Join(outDir, binaryName("gguard-setup", goos))
	if err := goBuild(root, goos, goarch, "./"+setupPkgDir, setupPath, ldflags); err != nil {
		return nil, err
	}

	// Ship the payload binaries next to the installer, so GopherGuard can also
	// be run without installing it.
	files := []string{setupPath}
	names, err := payloadBinaries(payload, goos)
	if err != nil {
		return nil, err
	}
	for _, name := range names {
		dst := filepath.Join(outDir, name)
		if err := copyFile(filepath.Join(payload, name), dst); err != nil {
			return nil, err
		}
		files = append(files, dst)
	}

	return files, nil
}

func buildDist(root string, t target) error {
	dir := filepath.Join(root, "dist", "build-"+t.goos+"-"+t.goarch)
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	defer os.RemoveAll(dir)

	fmt.Printf("Building %s-%s...\n", t.goos, t.goarch)
	files, err := buildSet(root, t.goos, t.goarch, dir)
	if err != nil {
		return err
	}

	// Only the installer is shipped, renamed for the platform it installs: it
	// already carries the binaries, so there is nothing left to wrap. Renaming
	// rather than copying also keeps the executable bit on Linux.
	asset := filepath.Join(root, "dist", t.asset)
	if err := os.Remove(asset); err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.Rename(files[0], asset)
}

// payloadBinaries lists the binaries buildSet put in payload for a platform.
func payloadBinaries(payload, goos string) ([]string, error) {
	names := []string{binaryName("gguard", goos)}
	if goos == "windows" {
		names = append(names, launcherName)
	}

	for _, name := range names {
		if _, err := os.Stat(filepath.Join(payload, name)); err != nil {
			return nil, fmt.Errorf("payload is missing %s", name)
		}
	}
	return names, nil
}

func binaryName(base, goos string) string {
	if goos == "windows" {
		return base + ".exe"
	}
	return base
}

// resetPayload empties the payload directory, leaving the tracked placeholder
// behind so the package keeps compiling on its own.
func resetPayload(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.Name() == payloadKeep {
			continue
		}
		if err := os.RemoveAll(filepath.Join(dir, e.Name())); err != nil {
			return err
		}
	}
	return nil
}

func goBuild(root, goos, goarch, pkg, out, flags string) error {
	cmd := exec.Command("go", "build", "-ldflags", flags, "-o", out, pkg)
	cmd.Dir = root
	cmd.Env = envWith(map[string]string{"GOOS": goos, "GOARCH": goarch})
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// stampResources writes the .syso files of every package that carries Windows
// resources, using the release version as the file and product version.
func stampResources(root, arch string) error {
	version, err := releaseVersion(root)
	if err != nil {
		return err
	}

	for _, pkg := range resourcePkgs {
		if err := runWinres(root, pkg, arch, winVersion(version)); err != nil {
			return fmt.Errorf("%s: %w", pkg, err)
		}
	}
	return nil
}

// runWinres builds the resources of one package. go-winres reads
// winres/winres.json and writes rsrc_windows_*.syso, both relative to the
// package directory, so it has to run from there.
func runWinres(root, pkg, arch, version string) error {
	if _, err := exec.LookPath("go-winres"); err != nil {
		return err
	}

	args := []string{"make", "--file-version", version, "--product-version", version}
	if arch != "" {
		args = append(args, "--arch", arch)
	}

	cmd := exec.Command("go-winres", args...)
	cmd.Dir = filepath.Join(root, pkg)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// releaseVersion reads the single source of truth for the version, so the
// resources can never drift from what "gguard version" prints.
func releaseVersion(root string) (string, error) {
	data, err := os.ReadFile(filepath.Join(root, versionFile))
	if err != nil {
		return "", err
	}

	m := regexp.MustCompile(`Version = "([^"]+)"`).FindStringSubmatch(string(data))
	if m == nil {
		return "", fmt.Errorf("no Version in %s", versionFile)
	}
	return winVersion(m[1]), nil
}

// winVersion pads a dotted version into the four-part form Windows resources
// expect, so "0.2.3" becomes "0.2.3.0".
func winVersion(v string) string {
	for strings.Count(v, ".") < 3 {
		v += ".0"
	}
	return v
}

func copyFile(src, dst string) error {
	s, err := os.Open(src)
	if err != nil {
		return err
	}
	defer s.Close()

	d, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(d, s); err != nil {
		d.Close()
		return err
	}
	return d.Close()
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
