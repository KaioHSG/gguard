// Package install knows where GopherGuard lives on this machine: the install
// directory, the service that runs it and the per-user tray setting. The main
// binary and the installer both use it, so they always agree on the answers.
package install

import (
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"

	"github.com/kardianos/service"
)

// Config describes a gguard service registration.
type Config struct {
	Name             string
	DisplayName      string
	Description      string
	Executable       string
	Arguments        []string
	WorkingDirectory string
	UserService      bool
}

// stub satisfies service.Interface. Registering, starting, stopping and
// removing a service never runs one, so everything except the service itself
// only needs a placeholder.
type stub struct{}

func (stub) Start(service.Service) error { return nil }
func (stub) Stop(service.Service) error  { return nil }

// New builds the service described by cfg without running it.
func New(cfg Config) (service.Service, error) {
	return NewFor(stub{}, cfg)
}

// NewFor builds the service described by cfg bound to prog, which is what the
// binary actually running the service has to pass.
func NewFor(prog service.Interface, cfg Config) (service.Service, error) {
	svcCfg := &service.Config{
		Name:             cfg.Name,
		DisplayName:      cfg.DisplayName,
		Description:      cfg.Description,
		Arguments:        cfg.Arguments,
		WorkingDirectory: cfg.WorkingDirectory,
	}
	if cfg.Executable != "" {
		svcCfg.Executable = cfg.Executable
	}
	if cfg.UserService {
		if u, err := user.Current(); err == nil {
			svcCfg.UserName = u.Username
		}
	}
	return service.New(prog, svcCfg)
}

// Control applies a service action: install, uninstall, start, stop or restart.
func Control(s service.Service, action string) error {
	if err := service.Control(s, action); err != nil {
		return fmt.Errorf("gguard %s: %w", action, err)
	}
	return nil
}

// ServiceExists reports whether a service with that name is registered. It
// works without administrator rights.
func ServiceExists(name string) bool {
	s, err := New(Config{Name: name, DisplayName: name})
	if err != nil {
		return false
	}
	_, err = s.Status()
	return err == nil
}

// Running reports whether the named service is registered and running. It is
// best-effort: an unknown service is reported as not running.
func Running(name string) (running bool, known bool) {
	s, err := New(Config{Name: name, DisplayName: name})
	if err != nil {
		return false, false
	}
	status, err := s.Status()
	if err != nil {
		return false, false
	}
	return status == service.StatusRunning, true
}

// RunningAny reports whether either gguard service is registered and running.
func RunningAny() (running bool, known bool) {
	if r, ok := Running("gguard"); ok {
		return r, true
	}
	return Running("gguard-user")
}

// DetectType reports which installation exists on this machine: the system
// service, the per-user service, or an install directory holding the binary.
func DetectType() (userService bool, installed bool) {
	if ServiceExists("gguard") {
		return false, true
	}
	if ServiceExists("gguard-user") {
		return true, true
	}

	if exists(filepath.Join(TargetDir(false), ExeName())) {
		return false, true
	}
	if exists(filepath.Join(TargetDir(true), ExeName())) {
		return true, true
	}
	return false, false
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// AbsPath resolves p against dir, leaving absolute paths alone.
func AbsPath(p, dir string) string {
	if filepath.IsAbs(p) {
		return p
	}
	abs, err := filepath.Abs(filepath.Join(dir, p))
	if err != nil {
		return p
	}
	return abs
}

// CopyFile copies src over dst, replacing it.
func CopyFile(src, dst string) error {
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

// CopyDir copies a directory tree recursively.
func CopyDir(src, dst string) error {
	if err := os.MkdirAll(dst, 0755); err != nil {
		return err
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, e := range entries {
		srcPath := filepath.Join(src, e.Name())
		dstPath := filepath.Join(dst, e.Name())

		if e.IsDir() {
			if err := CopyDir(srcPath, dstPath); err != nil {
				return err
			}
			continue
		}
		if err := CopyFile(srcPath, dstPath); err != nil {
			return err
		}
	}
	return nil
}
