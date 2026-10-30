//go:build windows

package main

import (
	"syscall"
	"unsafe"
)

// openWithDefault hands the target to the shell. ShellExecuteW is used instead
// of "cmd /c start" because cmd.exe is a console program: launching it from a
// process that has no console — the tray, the dashboard — would flash a black
// window every time the user clicks "Open log" or "Open dashboard".
func openWithDefault(target string) {
	const (
		swShowNormal = 1
	)

	shell32 := syscall.NewLazyDLL("shell32.dll")
	shellExecute := shell32.NewProc("ShellExecuteW")

	verb, err := syscall.UTF16PtrFromString("open")
	if err != nil {
		return
	}
	file, err := syscall.UTF16PtrFromString(target)
	if err != nil {
		return
	}

	shellExecute.Call(
		0,
		uintptr(unsafe.Pointer(verb)),
		uintptr(unsafe.Pointer(file)),
		0,
		0,
		swShowNormal,
	)
}
