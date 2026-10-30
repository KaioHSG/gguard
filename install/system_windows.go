//go:build windows

package install

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows/registry"
)

const (
	hwndBroadcast   = 0xFFFF
	wmSettingChange = 0x001A
	smtoAbortIfHung = 0x0002
)

var (
	user32                  = syscall.NewLazyDLL("user32.dll")
	procSendMessageTimeoutW = user32.NewProc("SendMessageTimeoutW")
)

func sendMessageTimeout(hWnd uintptr, msg uint32, wParam, lParam uintptr, flags uint32, timeout uint32) uintptr {
	ret, _, _ := procSendMessageTimeoutW.Call(hWnd, uintptr(msg), wParam, lParam, uintptr(flags), uintptr(timeout), 0)
	return ret
}

// AddToPath puts the install directory in the user or system PATH, once.
func AddToPath(targetDir string, system bool) error {
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
			log.Printf("%s is already in the %s PATH", targetDir, label)
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

	log.Printf("added %s to the %s PATH", targetDir, label)
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

// RemoveFromPath takes the install directory back out of the PATH.
func RemoveFromPath(targetDir string, system bool) error {
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

	log.Printf("removed %s from the %s PATH", targetDir, label)
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

// InstallStartMenuShortcut creates the Start Menu entry that opens the
// dashboard. It points at the windowless launcher, so nothing flashes.
func InstallStartMenuShortcut(targetDir string, system bool) error {
	dir := startMenuDir(system)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("cannot create the Start Menu directory: %w", err)
	}

	exePath := LauncherPath(filepath.Join(targetDir, ExeName()))
	shortcutPath := filepath.Join(dir, "GopherGuard.lnk")

	if err := createShellLink(exePath, shortcutPath, "gui", "File watcher and backup automator"); err != nil {
		return err
	}

	log.Printf("installed Start Menu shortcut: %s", shortcutPath)
	return nil
}

// RemoveStartMenuShortcut deletes the Start Menu entry.
func RemoveStartMenuShortcut(system bool) error {
	dir := startMenuDir(system)
	shortcutPath := filepath.Join(dir, "GopherGuard.lnk")
	if err := os.Remove(shortcutPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	os.Remove(dir)
	log.Printf("removed Start Menu shortcut: %s", shortcutPath)
	return nil
}

// InstallDesktopEntry is a no-op on Windows, which uses shortcuts instead.
func InstallDesktopEntry(_ string, _ bool) error { return nil }

// RemoveDesktopEntry is a no-op on Windows.
func RemoveDesktopEntry(_ bool) error { return nil }

// InstallIcon is a no-op on Windows: the icon is baked into the executable.
func InstallIcon(_ bool) error { return nil }

// RemoveIcon is a no-op on Windows.
func RemoveIcon(_ bool) error { return nil }

func createShellLink(targetPath, shortcutPath, args, description string) error {
	cmd := fmt.Sprintf(
		`$ws=New-Object -ComObject WScript.Shell;$s=$ws.CreateShortcut('%s');$s.TargetPath='%s';$s.Arguments='%s';$s.Description='%s';$s.Save()`,
		shortcutPath, targetPath, strings.ReplaceAll(args, "'", "''"), strings.ReplaceAll(description, "'", "''"),
	)
	return exec.Command("powershell", "-NoProfile", "-Command", cmd).Run()
}

// InstallAppsRegistry registers GopherGuard in Apps & Features. Uninstalling
// runs the app itself, which is always present in the install directory.
func InstallAppsRegistry(targetDir string, system bool) error {
	rootKey := registry.CURRENT_USER
	keyPath := `Software\Microsoft\Windows\CurrentVersion\Uninstall\GopherGuard`
	if system {
		rootKey = registry.LOCAL_MACHINE
		keyPath = `SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\GopherGuard`
	}

	access := uint32(registry.QUERY_VALUE | registry.SET_VALUE)
	key, _, err := registry.CreateKey(rootKey, keyPath, access)
	if err != nil {
		return fmt.Errorf("cannot create the uninstall registry key: %w", err)
	}
	defer key.Close()

	exePath := filepath.Join(targetDir, ExeName())

	_ = key.SetStringValue("DisplayName", "GopherGuard")
	_ = key.SetStringValue("DisplayVersion", Version)
	_ = key.SetStringValue("Publisher", "KaioHSG")
	_ = key.SetStringValue("UninstallString", fmt.Sprintf(`"%s" uninstall`, exePath))
	_ = key.SetStringValue("InstallLocation", targetDir)
	_ = key.SetStringValue("DisplayIcon", exePath)
	_ = key.SetStringValue("HelpLink", "https://github.com/KaioHSG/gguard")
	_ = key.SetStringValue("URLInfoAbout", "https://github.com/KaioHSG/gguard")

	log.Printf("registered %s in Apps & Features", targetDir)
	return nil
}

// RemoveAppsRegistry drops the Apps & Features entry.
func RemoveAppsRegistry(system bool) error {
	rootKey := registry.CURRENT_USER
	keyPath := `Software\Microsoft\Windows\CurrentVersion\Uninstall\GopherGuard`
	if system {
		rootKey = registry.LOCAL_MACHINE
		keyPath = `SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\GopherGuard`
	}

	if err := registry.DeleteKey(rootKey, keyPath); err != nil && err != registry.ErrNotExist {
		return fmt.Errorf("cannot remove the uninstall registry key: %w", err)
	}

	log.Println("removed the Apps & Features entry")
	return nil
}

// scheduleRemoveOnReboot asks Windows to delete a leftover path on the next
// boot, which is the only way to remove the executable that is running.
func scheduleRemoveOnReboot(path string) {
	const moveFileDelayUntilReboot = 0x4

	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	procMoveFileEx := kernel32.NewProc("MoveFileExW")

	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return
	}
	procMoveFileEx.Call(uintptr(unsafe.Pointer(p)), 0, moveFileDelayUntilReboot)
}
