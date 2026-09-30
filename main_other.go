//go:build !windows

package main

import (
	"os"
	"path/filepath"
)

func isServiceManager() bool {
	return false
}

func activeUserConfigDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".local", "bin")
}

func userConfigDir() string {
	return installTargetDir(true)
}