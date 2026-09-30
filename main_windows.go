//go:build windows

package main

import (
	"path/filepath"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows/svc"
)

func isServiceManager() bool {
	isService, err := svc.IsWindowsService()
	if err != nil {
		return false
	}
	return isService
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
	return installTargetDir(true)
}