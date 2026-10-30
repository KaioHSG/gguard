//go:build !windows

package install

import (
	"os"
	"path/filepath"
)

// TargetDir is where GopherGuard installs its binary for the given mode.
func TargetDir(userService bool) string {
	if userService {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, ".local", "bin")
	}
	return "/usr/local/bin"
}

// ExeName is the name of the installed binary.
func ExeName() string {
	return "gguard"
}

// LauncherName is empty off Windows: nothing needs to hide a console there.
func LauncherName() string {
	return ""
}

// LauncherPath is the app itself off Windows.
func LauncherPath(appPath string) string {
	return appPath
}

// IsAdmin reports whether the process is running as root.
func IsAdmin() bool {
	return os.Geteuid() == 0
}

// EnableTrayAutostart is a no-op off Windows: there is no tray and no logon
// entry to write. The tray setting therefore always reads as off.
func EnableTrayAutostart(_ string) error { return nil }

// TrayEnabled reports whether the tray is turned on. Never off Windows.
func TrayEnabled() bool { return false }

// DisableTrayAutostart is a no-op off Windows.
func DisableTrayAutostart() error { return nil }
