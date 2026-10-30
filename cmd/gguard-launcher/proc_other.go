//go:build !windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// appPath is gguard, expected next to this launcher.
func appPath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}

	path := filepath.Join(filepath.Dir(exe), "gguard")
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("cannot find gguard next to gguard-launcher")
	}
	return path, nil
}

// startDetached runs gguard on its own, so it outlives this launcher. The
// launcher is only shipped on Windows, where hiding a console is the point;
// elsewhere this keeps the package building and behaving sensibly.
func startDetached(app string, args []string) error {
	cmd := exec.Command(app, args...)
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

// notify reports a problem on the standard error, since there is no message box
// off Windows.
func notify(msg string) {
	fmt.Fprintf(os.Stderr, "gguard-launcher: %s\n", msg)
}
