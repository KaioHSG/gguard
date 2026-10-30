//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// startElevated re-runs this installer through the UAC prompt, so a
// double-clicked installer can still do a system-wide install.
func startElevated(args []string) bool {
	exe, err := os.Executable()
	if err != nil {
		return false
	}

	vbs := fmt.Sprintf(
		"Set UAC = CreateObject(\"Shell.Application\")\nUAC.ShellExecute \"%s\", \"%s\", \"\", \"RunAs\", 1",
		strings.ReplaceAll(exe, "\"", "\"\""),
		strings.ReplaceAll(strings.Join(args, " "), "\"", "\"\""),
	)

	tmpFile := filepath.Join(os.Getenv("TEMP"), "gguard_elevate.vbs")
	if err := os.WriteFile(tmpFile, []byte(vbs), 0644); err != nil {
		return false
	}
	defer os.Remove(tmpFile)

	return exec.Command("cscript.exe", "//NoLogo", tmpFile).Run() == nil
}
