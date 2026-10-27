package main

import (
	"os/exec"
	"runtime"
)

// openURL opens a URL with the default browser.
func openURL(url string) {
	if url == "" {
		return
	}
	openWithDefault(url)
}

// openPath opens a file or folder with the default handler.
func openPath(path string) {
	if path == "" {
		return
	}
	openWithDefault(path)
}

func openWithDefault(target string) {
	switch runtime.GOOS {
	case "windows":
		_ = exec.Command("cmd", "/c", "start", "", target).Start()
	case "darwin":
		_ = exec.Command("open", target).Start()
	default:
		_ = exec.Command("xdg-open", target).Start()
	}
}
