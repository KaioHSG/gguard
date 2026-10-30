//go:build !windows

package main

import (
	"os"
	"path/filepath"

	"github.com/KaioHSG/gguard/install"
)

func isServiceManager() bool {
	return false
}

// consoleAvailable reports whether gguard can talk to the user through the
// terminal it was started from, so a bare run from a shell prints usage while a
// desktop launcher (Terminal=false) gets the dashboard instead.
func consoleAvailable() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

func activeUserConfigDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".local", "bin")
}

func userConfigDir() string {
	return install.TargetDir(true)
}
