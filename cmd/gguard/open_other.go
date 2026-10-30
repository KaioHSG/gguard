//go:build !windows

package main

import (
	"os/exec"
	"runtime"
)

// openWithDefault hands the target to the desktop's default handler.
func openWithDefault(target string) {
	if runtime.GOOS == "darwin" {
		_ = exec.Command("open", target).Start()
		return
	}
	_ = exec.Command("xdg-open", target).Start()
}
