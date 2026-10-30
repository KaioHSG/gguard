//go:build windows

package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	scManagerConnect   = 0x0001
	serviceQueryStatus = 0x0004
	svcStateRunning    = 0x00000004
)

type winServiceStatus struct {
	ServiceType      uint32
	CurrentState     uint32
	ControlsAccepted uint32
	Win32ExitCode    uint32
	ServiceExitCode  uint32
	CheckPoint       uint32
	WaitHint         uint32
}

var (
	advapi32             = syscall.NewLazyDLL("advapi32.dll")
	procOpenSCManagerW   = advapi32.NewProc("OpenSCManagerW")
	procOpenServiceW     = advapi32.NewProc("OpenServiceW")
	procQueryServiceStat = advapi32.NewProc("QueryServiceStatus")
	procCloseServiceHnd  = advapi32.NewProc("CloseServiceHandle")

	user32          = syscall.NewLazyDLL("user32.dll")
	procMessageBoxW = user32.NewProc("MessageBoxW")
)

const (
	mbOK        = 0x00000000
	mbIconError = 0x00000010
)

// fatalUserError reports a fatal error and exits. gguard runs without a
// console most of the time (double-click, Start Menu shortcut, logon entry), so
// a failure is shown in a message box instead of vanishing.
func fatalUserError(format string, args ...any) {
	msg := fmt.Sprintf("gguard: "+format, args...)
	log.Print(msg)

	if consoleAvailable() {
		os.Exit(1)
	}

	title, _ := syscall.UTF16PtrFromString("GopherGuard")
	text, _ := syscall.UTF16PtrFromString(msg)
	procMessageBoxW.Call(0, uintptr(unsafe.Pointer(text)), uintptr(unsafe.Pointer(title)), mbOK|mbIconError)
	os.Exit(1)
}

// serviceRunning reports whether a Windows service exists and is running. It
// opens the service manager with minimal access so it works without
// administrator rights (unlike kardianos/service's Status).
func serviceRunning(name string) (running bool, known bool) {
	namePtr, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return false, false
	}

	scm, _, _ := procOpenSCManagerW.Call(0, 0, uintptr(scManagerConnect))
	if scm == 0 {
		return false, false
	}
	defer procCloseServiceHnd.Call(scm)

	svc, _, _ := procOpenServiceW.Call(scm, uintptr(unsafe.Pointer(namePtr)), uintptr(serviceQueryStatus))
	if svc == 0 {
		return false, false
	}
	defer procCloseServiceHnd.Call(svc)

	var status winServiceStatus
	ret, _, _ := procQueryServiceStat.Call(svc, uintptr(unsafe.Pointer(&status)))
	if ret == 0 {
		return false, false
	}
	return status.CurrentState == svcStateRunning, true
}

// spawnDetached starts a new process that outlives this one, with no console.
func spawnDetached(exe string, args []string) (int, error) {
	cmd := exec.Command(exe, args...)
	cmd.Dir = filepath.Dir(exe)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: windows.DETACHED_PROCESS | windows.CREATE_NEW_PROCESS_GROUP,
	}
	if err := cmd.Start(); err != nil {
		return 0, err
	}
	pid := cmd.Process.Pid
	_ = cmd.Process.Release()
	return pid, nil
}

// processAlive reports whether a process with the given pid is still running.
func processAlive(pid int) bool {
	const processQueryLimitedInformation = 0x1000
	h, err := windows.OpenProcess(processQueryLimitedInformation, false, uint32(pid))
	if err != nil {
		return false
	}
	windows.CloseHandle(h)
	return true
}

// killProcess terminates the process with the given pid.
func killProcess(pid int) error {
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return p.Kill()
}
