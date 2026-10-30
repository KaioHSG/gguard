//go:build !windows

package main

import (
	"os"
	"os/exec"
)

// startElevated re-runs this installer with pkexec or sudo, so a launcher can
// still do a system-wide install.
func startElevated(args []string) bool {
	exe, err := os.Executable()
	if err != nil {
		return false
	}

	fullArgs := append([]string{exe}, args...)
	return exec.Command("pkexec", fullArgs...).Run() == nil ||
		exec.Command("sudo", fullArgs...).Run() == nil
}
