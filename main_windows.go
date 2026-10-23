//go:build windows

package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

// isTerminal reports whether gguard was launched from an existing console (a
// shell) rather than a double-click that owns its own console window. It uses
// GetConsoleProcessList: a shared console means a parent shell is attached.
func isTerminal() bool {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	getConsoleProcessList := kernel32.NewProc("GetConsoleProcessList")

	var pids [64]uint32
	n, _, _ := getConsoleProcessList.Call(
		uintptr(unsafe.Pointer(&pids[0])),
		uintptr(len(pids)),
	)
	return n > 1
}

func startElevated(args []string) bool {
	exe := filepath.Join(exeDir(), exeName())
	argStr := strings.Join(args, " ")

	vbs := fmt.Sprintf(
		"Set UAC = CreateObject(\"Shell.Application\")\nUAC.ShellExecute \"%s\", \"%s\", \"\", \"RunAs\", 1",
		strings.ReplaceAll(exe, "\"", "\"\""),
		strings.ReplaceAll(argStr, "\"", "\"\""),
	)

	tmpFile := filepath.Join(os.Getenv("TEMP"), "gguard_elevate.vbs")
	os.WriteFile(tmpFile, []byte(vbs), 0644)
	defer os.Remove(tmpFile)

	return exec.Command("cscript.exe", "//NoLogo", tmpFile).Run() == nil
}

func showInstallDialog() {
	_, installed := detectInstallType()

	fmt.Println("=== GopherGuard Installer ===")
	fmt.Println()

	bufr := bufio.NewReader(os.Stdin)

	if installed {
		fmt.Println("GopherGuard is already installed.")
		fmt.Println("[U] Upgrade")
		fmt.Println("[C] Cancel")
		fmt.Print("Choice: ")

		lineb, _, errl := bufr.ReadLine()
		if errl == io.EOF || errl != nil {
			return
		}
		choice := strings.TrimSpace(string(lineb))

		if strings.HasPrefix(strings.ToLower(choice), "u") {
			doUpgrade()
		}
	} else {
		fmt.Println("GopherGuard is not installed.")
		fmt.Println("[A] Admin install")
		fmt.Println("[U] User install")
		fmt.Println("[C] Cancel")
		fmt.Print("Choice: ")

		lineb, _, errl := bufr.ReadLine()
		if errl == io.EOF || errl != nil {
			return
		}
		choice := strings.TrimSpace(string(lineb))
		lower := strings.ToLower(choice)

		if strings.HasPrefix(lower, "a") {
			if !isAdmin() {
				fmt.Println("Requesting administrator privileges...")
				startElevated([]string{"install"})
				return
			}
			doInstall(false)
		} else if strings.HasPrefix(lower, "u") {
			doInstall(true)
		}
	}

	pauseBeforeExit()
}

func pauseBeforeExit() {
	fmt.Println()
	fmt.Print("Press Enter to close...")
	bufr := bufio.NewReader(os.Stdin)
	bufr.ReadLine()
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
