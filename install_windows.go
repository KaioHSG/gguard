//go:build windows

package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const (
	hwndBroadcast   = 0xFFFF
	wmSettingChange = 0x001A
	smtoAbortIfHung = 0x0002
)

var (
	user32                     = syscall.NewLazyDLL("user32.dll")
	procSendMessageTimeoutW    = user32.NewProc("SendMessageTimeoutW")
)

func sendMessageTimeout(hWnd uintptr, msg uint32, wParam, lParam uintptr, flags uint32, timeout uint32) uintptr {
	ret, _, _ := procSendMessageTimeoutW.Call(hWnd, uintptr(msg), wParam, lParam, uintptr(flags), uintptr(timeout), 0)
	return ret
}

func installTargetDir(userService bool) string {
	if userService {
		localAppData, _ := os.UserConfigDir()
		return filepath.Join(localAppData, "Programs", "GopherGuard")
	}
	return filepath.Join(os.Getenv("ProgramFiles"), "GopherGuard")
}

func exeName() string {
	return "gguard.exe"
}

func isAdmin() bool {
	var sid *windows.SID
	err := windows.AllocateAndInitializeSid(
		&windows.SECURITY_NT_AUTHORITY, 2,
		windows.SECURITY_BUILTIN_DOMAIN_RID,
		windows.DOMAIN_ALIAS_RID_ADMINS,
		0, 0, 0, 0, 0, 0, &sid,
	)
	if err != nil {
		return false
	}
	defer windows.FreeSid(sid)

	token := windows.Token(0)
	isMember, _ := token.IsMember(sid)
	return isMember
}

func addToPath(targetDir string, system bool) error {
	var keyPath string
	var rootKey registry.Key
	access := uint32(registry.QUERY_VALUE | registry.SET_VALUE)

	if system {
		rootKey = registry.LOCAL_MACHINE
		keyPath = `SYSTEM\CurrentControlSet\Control\Session Manager\Environment`
	} else {
		rootKey = registry.CURRENT_USER
		keyPath = `Environment`
	}

	key, err := registry.OpenKey(rootKey, keyPath, access)
	if err != nil {
		return fmt.Errorf("cannot open PATH registry key: %w", err)
	}
	defer key.Close()

	existing, _, err := key.GetStringValue("Path")
	if err != nil && err != registry.ErrNotExist {
		return fmt.Errorf("cannot read PATH: %w", err)
	}

	for _, entry := range filepath.SplitList(existing) {
		if winPathEqual(entry, targetDir) {
			return nil
		}
	}

	newPath := existing
	if newPath != "" {
		newPath += ";" + targetDir
	} else {
		newPath = targetDir
	}

	if err := key.SetStringValue("Path", newPath); err != nil {
		return fmt.Errorf("cannot update PATH: %w", err)
	}

	label := "user"
	if system {
		label = "system"
	}
	log.Printf("added %s to %s PATH", targetDir, label)
	broadcastEnvChange()
	return nil
}

func winPathEqual(a, b string) bool {
	absA, _ := filepath.Abs(a)
	absB, _ := filepath.Abs(b)
	return strings.EqualFold(absA, absB)
}

func broadcastEnvChange() {
	envPtr, _ := syscall.UTF16PtrFromString("Environment")
	sendMessageTimeout(
		hwndBroadcast,
		wmSettingChange,
		0,
		uintptr(unsafe.Pointer(envPtr)),
		smtoAbortIfHung,
		5000,
	)
}

func removeFromPath(targetDir string, system bool) error {
	var keyPath string
	var rootKey registry.Key
	access := uint32(registry.QUERY_VALUE | registry.SET_VALUE)

	if system {
		rootKey = registry.LOCAL_MACHINE
		keyPath = `SYSTEM\CurrentControlSet\Control\Session Manager\Environment`
	} else {
		rootKey = registry.CURRENT_USER
		keyPath = `Environment`
	}

	key, err := registry.OpenKey(rootKey, keyPath, access)
	if err != nil {
		return err
	}
	defer key.Close()

	existing, _, err := key.GetStringValue("Path")
	if err != nil {
		return err
	}

	cleanTarget := winAbsClean(targetDir)
	var keep []string
	for _, entry := range filepath.SplitList(existing) {
		if !strings.EqualFold(winAbsClean(entry), cleanTarget) {
			keep = append(keep, entry)
		}
	}

	newPath := strings.Join(keep, ";")
	if err := key.SetStringValue("Path", newPath); err != nil {
		return fmt.Errorf("cannot update PATH: %w", err)
	}

	label := "user"
	if system {
		label = "system"
	}
	log.Printf("removed %s from %s PATH", targetDir, label)
	broadcastEnvChange()
	return nil
}

func winAbsClean(p string) string {
	abs, _ := filepath.Abs(p)
	return filepath.Clean(abs)
}

func startMenuDir(system bool) string {
	if system {
		return filepath.Join(os.Getenv("ProgramData"), "Microsoft", "Windows", "Start Menu", "Programs", "GopherGuard")
	}
	return filepath.Join(os.Getenv("APPDATA"), "Microsoft", "Windows", "Start Menu", "Programs", "GopherGuard")
}

func installStartMenuShortcut(targetDir string, system bool) error {
	dir := startMenuDir(system)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("cannot create start menu directory: %w", err)
	}

	exePath := filepath.Join(targetDir, exeName())
	shortcutPath := filepath.Join(dir, "GopherGuard.lnk")

	if err := createShellLink(exePath, shortcutPath, "File watcher and backup automator"); err != nil {
		return err
	}

	log.Printf("installed start menu shortcut: %s", shortcutPath)
	return nil
}

func uninstallStartMenuShortcut(system bool) error {
	dir := startMenuDir(system)
	shortcutPath := filepath.Join(dir, "GopherGuard.lnk")
	if err := os.Remove(shortcutPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	os.Remove(dir)
	log.Printf("removed start menu shortcut: %s", shortcutPath)
	return nil
}

func installDesktopEntry(_ string, _ bool) error { return nil }
func uninstallDesktopEntry(_ bool) error          { return nil }
func installIcon(_ bool) error                     { return nil }
func uninstallIcon(_ bool) error                   { return nil }

func createShellLink(targetPath, shortcutPath, description string) error {
	ole32 := syscall.NewLazyDLL("ole32.dll")
	_ = syscall.NewLazyDLL("shell32.dll")

	clsidLink, err := windows.GUIDFromString("{00021401-0000-0000-C000-000000000046}")
	if err != nil {
		return err
	}
	iidLink, err := windows.GUIDFromString("{000214F9-0000-0000-C000-000000000046}")
	if err != nil {
		return err
	}
	iidPersist, err := windows.GUIDFromString("{0000010B-0000-0000-C000-000000000046}")
	if err != nil {
		return err
	}

	procCoInitializeEx := ole32.NewProc("CoInitializeEx")
	ret, _, _ := procCoInitializeEx.Call(0, 2)
	if ret != 0 && ret != 1 {
		return fmt.Errorf("CoInitializeEx failed: 0x%x", ret)
	}
	defer ole32.NewProc("CoUninitialize").Call()

	procCoCreateInstance := ole32.NewProc("CoCreateInstance")
	var pLink uintptr
	ret, _, _ = procCoCreateInstance.Call(
		uintptr(unsafe.Pointer(&clsidLink)),
		0,
		1,
		uintptr(unsafe.Pointer(&iidLink)),
		uintptr(unsafe.Pointer(&pLink)),
	)
	if ret != 0 {
		return fmt.Errorf("CoCreateInstance failed: 0x%x", ret)
	}
	defer vcall(pLink, 2)

	exePtr, _ := syscall.UTF16PtrFromString(targetPath)
	vcall(pLink, 20, uintptr(unsafe.Pointer(exePtr)))

	descPtr, _ := syscall.UTF16PtrFromString(description)
	vcall(pLink, 7, uintptr(unsafe.Pointer(descPtr)))

	iconPtr, _ := syscall.UTF16PtrFromString(targetPath)
	vcall(pLink, 17, uintptr(unsafe.Pointer(iconPtr)), 0)

	var pPersist uintptr
	vcall(pLink, 0, uintptr(unsafe.Pointer(&iidPersist)), uintptr(unsafe.Pointer(&pPersist)))
	if pPersist == 0 {
		return fmt.Errorf("IPersistFile interface not supported")
	}
	defer vcall(pPersist, 2)

	pathPtr, _ := syscall.UTF16PtrFromString(shortcutPath)
	ret = vcall(pPersist, 6, uintptr(unsafe.Pointer(pathPtr)), 1)
	if ret != 0 {
		return fmt.Errorf("IPersistFile::Save failed: 0x%x", ret)
	}

	return nil
}

func vcall(obj uintptr, methodIndex int, args ...uintptr) uintptr {
	vtbl := *(*uintptr)(unsafe.Pointer(obj))
	offset := unsafe.Sizeof(uintptr(0)) * uintptr(methodIndex)
	method := *(*uintptr)(unsafe.Add(unsafe.Pointer(vtbl), int(offset)))

	switch len(args) {
	case 0:
		ret, _, _ := syscall.SyscallN(method, 1, obj)
		return ret
	case 1:
		ret, _, _ := syscall.SyscallN(method, 2, obj, args[0])
		return ret
	case 2:
		ret, _, _ := syscall.SyscallN(method, 3, obj, args[0], args[1])
		return ret
	case 3:
		ret, _, _ := syscall.SyscallN(method, 4, obj, args[0], args[1], args[2])
		return ret
	default:
		return 0
	}
}