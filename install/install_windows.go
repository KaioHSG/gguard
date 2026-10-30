//go:build windows

package install

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// TargetDir is where GopherGuard installs its binary for the given mode.
func TargetDir(userService bool) string {
	if userService {
		localAppData, _ := os.UserConfigDir()
		return filepath.Join(localAppData, "Programs", "GopherGuard")
	}
	return filepath.Join(os.Getenv("ProgramFiles"), "GopherGuard")
}

// ExeName is the name of the installed binary.
func ExeName() string {
	return "gguard.exe"
}

// LauncherName is the windowless launcher that starts the app for the GUI entry
// points (logon entry, Start Menu shortcut).
func LauncherName() string {
	return "gguard-launcher.exe"
}

// LauncherPath returns the launcher that sits next to the app, or the app path
// itself when there is none — an installation from an older version, for
// example.
func LauncherPath(appPath string) string {
	launcher := filepath.Join(filepath.Dir(appPath), LauncherName())
	if exists(launcher) {
		return launcher
	}
	return appPath
}

// IsAdmin reports whether the process is running with administrator rights.
func IsAdmin() bool {
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

const (
	trayRunKey   = `Software\Microsoft\Windows\CurrentVersion\Run`
	trayRunValue = "GopherGuardTray"
)

// EnableTrayAutostart registers gguard to start the tray when the user logs in,
// which is what makes 'tray --on' persist across reboots.
func EnableTrayAutostart(exePath string) error {
	key, _, err := registry.CreateKey(registry.CURRENT_USER, trayRunKey, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("cannot open Run key: %w", err)
	}
	defer key.Close()

	return key.SetStringValue(trayRunValue, `"`+exePath+`" tray -q`)
}

// TrayEnabled reports whether the tray is turned on. It is the "tray on/off"
// setting, so every entry point (logon, Start Menu shortcut, 'gui') agrees on
// whether a tray icon should show. EnableTrayAutostart and DisableTrayAutostart
// write it.
func TrayEnabled() bool {
	key, err := registry.OpenKey(registry.CURRENT_USER, trayRunKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer key.Close()

	_, _, err = key.GetStringValue(trayRunValue)
	return err == nil
}

// DisableTrayAutostart removes the logon entry.
func DisableTrayAutostart() error {
	key, err := registry.OpenKey(registry.CURRENT_USER, trayRunKey, registry.SET_VALUE)
	if err != nil {
		return nil
	}
	defer key.Close()

	if err := key.DeleteValue(trayRunValue); err != nil && err != registry.ErrNotExist {
		return err
	}
	return nil
}
