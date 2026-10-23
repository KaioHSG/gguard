//go:build !windows

// Package notify shows desktop notifications in a best-effort way. Failures are
// reported to the caller but are never fatal.
package notify

import (
	"os/exec"
	"runtime"
	"strings"
)

// Notify displays a desktop notification.
func Notify(title, message string) error {
	if !enabled {
		return nil
	}
	if runtime.GOOS == "darwin" {
		script := "display notification " + appleQuote(message) + " with title " + appleQuote(title)
		return exec.Command("osascript", "-e", script).Run()
	}
	return exec.Command("notify-send", title, message).Run()
}

// appleQuote escapes a string for use inside an AppleScript literal.
func appleQuote(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}
