//go:build !windows

package main

import (
	"os"
	"time"
)

func processRunning(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return p.Signal(os.Signal(nil)) == nil
}

func processUptime(pid int) time.Duration {
	return 0
}