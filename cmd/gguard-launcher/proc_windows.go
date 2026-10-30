//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// appPath is gguard, expected next to this launcher.
func appPath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}

	path := filepath.Join(filepath.Dir(exe), "gguard.exe")
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("cannot find gguard.exe next to gguard-launcher.exe")
	}
	return path, nil
}

// startDetached runs gguard with no console at all, so nothing appears on
// screen and the process outlives this launcher.
func startDetached(app string, args []string) error {
	cmd := exec.Command(app, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: windows.DETACHED_PROCESS | windows.CREATE_NEW_PROCESS_GROUP,
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

// notify shows a message box: this process has no console, so there is nowhere
// else to report a problem.
func notify(msg string) {
	const (
		mbOK        = 0x00000000
		mbIconError = 0x00000010
	)

	user32 := syscall.NewLazyDLL("user32.dll")
	messageBox := user32.NewProc("MessageBoxW")

	title, _ := syscall.UTF16PtrFromString("GopherGuard")
	text, _ := syscall.UTF16PtrFromString(msg)
	messageBox.Call(0, uintptr(unsafe.Pointer(text)), uintptr(unsafe.Pointer(title)), mbOK|mbIconError)
}
