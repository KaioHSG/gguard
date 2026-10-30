//go:build windows

package main

import (
	"path/filepath"
	"syscall"
	"unsafe"

	"github.com/KaioHSG/gguard/install"
	"golang.org/x/sys/windows/svc"
)

func isServiceManager() bool {
	isService, err := svc.IsWindowsService()
	if err != nil {
		return false
	}
	return isService
}

// consoleAvailable reports whether gguard can talk to the user through a
// terminal. The binary is built without a console (-H windowsgui), so the only
// way to get one is to be started from a terminal, directly or through
// gguard-cli. Everything else — a double-click, the Start Menu shortcut, the
// logon entry, the service — runs without a console and keeps its output in
// gguard.log.
func consoleAvailable() bool {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	getConsoleWindow := kernel32.NewProc("GetConsoleWindow")

	hwnd, _, _ := getConsoleWindow.Call()
	return hwnd != 0
}

func activeUserConfigDir() string {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	procWTSGetActiveConsoleSessionId := kernel32.NewProc("WTSGetActiveConsoleSessionId")
	sessionId, _, _ := procWTSGetActiveConsoleSessionId.Call()
	if sessionId == 0xFFFFFFFF {
		return ""
	}

	wtsapi32 := syscall.NewLazyDLL("wtsapi32.dll")
	procWTSQueryUserToken := wtsapi32.NewProc("WTSQueryUserToken")
	var token syscall.Token
	ret, _, _ := procWTSQueryUserToken.Call(sessionId, uintptr(unsafe.Pointer(&token)))
	if ret == 0 {
		return ""
	}
	defer token.Close()

	userenv := syscall.NewLazyDLL("userenv.dll")
	procGetUserProfileDirectory := userenv.NewProc("GetUserProfileDirectoryW")

	var size uint32
	procGetUserProfileDirectory.Call(uintptr(token), 0, uintptr(unsafe.Pointer(&size)))

	buf := make([]uint16, size)
	ret, _, _ = procGetUserProfileDirectory.Call(uintptr(token), uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)))
	if ret == 0 {
		return ""
	}

	profilePath := syscall.UTF16ToString(buf)
	if profilePath == "" {
		return ""
	}

	return filepath.Join(profilePath, "AppData", "Roaming", "Programs", "GopherGuard")
}

func userConfigDir() string {
	return install.TargetDir(true)
}
