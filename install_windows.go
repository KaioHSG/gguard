//go:build windows

package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
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

	label := "user"
	if system {
		label = "system"
		rootKey = registry.LOCAL_MACHINE
		keyPath = `SYSTEM\CurrentControlSet\Control\Session Manager\Environment`
	} else {
		rootKey = registry.CURRENT_USER
		keyPath = `Environment`
	}

	key, err := registry.OpenKey(rootKey, keyPath, access)
	if err != nil {
		return fmt.Errorf("cannot open %s PATH registry key: %w", label, err)
	}
	defer key.Close()

	targetAbs, _ := filepath.Abs(targetDir)

	existing, valType, err := key.GetStringValue("Path")
	if err != nil && err != registry.ErrNotExist {
		return fmt.Errorf("cannot read %s PATH: %w", label, err)
	}

	for _, entry := range filepath.SplitList(existing) {
		if winPathEqual(entry, targetAbs) {
			log.Printf("%s is already in %s PATH", targetDir, label)
			return nil
		}
	}

	newPath := existing
	if newPath != "" {
		newPath += ";"
	}
	newPath += targetAbs

	if valType == registry.EXPAND_SZ || system {
		if err := key.SetExpandStringValue("Path", newPath); err != nil {
			return fmt.Errorf("cannot update %s PATH: %w", label, err)
		}
	} else {
		if err := key.SetStringValue("Path", newPath); err != nil {
			return fmt.Errorf("cannot update %s PATH: %w", label, err)
		}
	}

	fmt.Fprintf(os.Stderr, "gguard: added %s to %s PATH\n", targetDir, label)
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
	label := "user"
	rootKey := registry.CURRENT_USER
	keyPath := `Environment`
	if system {
		label = "system"
		rootKey = registry.LOCAL_MACHINE
		keyPath = `SYSTEM\CurrentControlSet\Control\Session Manager\Environment`
	}

	access := uint32(registry.QUERY_VALUE | registry.SET_VALUE)
	key, err := registry.OpenKey(rootKey, keyPath, access)
	if err != nil {
		return fmt.Errorf("cannot open %s PATH registry key: %w", label, err)
	}
	defer key.Close()

	existing, valType, err := key.GetStringValue("Path")
	if err != nil {
		return fmt.Errorf("cannot read %s PATH: %w", label, err)
	}

	cleanTarget := winAbsClean(targetDir)
	var keep []string
	for _, entry := range filepath.SplitList(existing) {
		if !strings.EqualFold(winAbsClean(entry), cleanTarget) {
			keep = append(keep, entry)
		}
	}

	newPath := strings.Join(keep, ";")

	if valType == registry.EXPAND_SZ || system {
		if err := key.SetExpandStringValue("Path", newPath); err != nil {
			return fmt.Errorf("cannot update %s PATH: %w", label, err)
		}
	} else {
		if err := key.SetStringValue("Path", newPath); err != nil {
			return fmt.Errorf("cannot update %s PATH: %w", label, err)
		}
	}

	fmt.Fprintf(os.Stderr, "gguard: removed %s from %s PATH\n", targetDir, label)
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
	cmd := fmt.Sprintf(
		`$ws=New-Object -ComObject WScript.Shell;$s=$ws.CreateShortcut('%s');$s.TargetPath='%s';$s.Description='%s';$s.Save()`,
		shortcutPath, targetPath, strings.ReplaceAll(description, "'", "''"),
	)
	return exec.Command("powershell", "-NoProfile", "-Command", cmd).Run()
}