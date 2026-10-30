//go:build !windows

package install

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// AddToPath only reminds the user: the installer cannot edit a Unix PATH from
// a script it does not own.
func AddToPath(targetDir string, _ bool) error {
	log.Printf("gguard installed to %s (make sure it is in your PATH)", targetDir)
	return nil
}

// RemoveFromPath has nothing to undo off Windows.
func RemoveFromPath(_ string, _ bool) error { return nil }

func desktopDir(system bool) string {
	if system {
		return "/usr/share/applications"
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "applications")
}

func iconDir(system bool) string {
	if system {
		return "/usr/share/icons/hicolor/256x256/apps"
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "icons", "hicolor", "256x256", "apps")
}

// InstallDesktopEntry writes the .desktop launcher, which opens the dashboard.
func InstallDesktopEntry(targetDir string, system bool) error {
	data, err := resources.ReadFile("resources/gguard.desktop")
	if err != nil {
		return fmt.Errorf("cannot read the embedded desktop entry: %w", err)
	}

	exePath := filepath.Join(targetDir, ExeName())
	content := strings.ReplaceAll(string(data), "{exec}", exePath)

	dir := desktopDir(system)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("cannot create the desktop directory %s: %w", dir, err)
	}

	desktopPath := filepath.Join(dir, "gguard.desktop")
	if err := os.WriteFile(desktopPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("cannot write the desktop entry: %w", err)
	}

	log.Printf("installed desktop entry: %s", desktopPath)
	return nil
}

// RemoveDesktopEntry deletes the .desktop launcher.
func RemoveDesktopEntry(system bool) error {
	path := filepath.Join(desktopDir(system), "gguard.desktop")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	log.Printf("removed desktop entry: %s", path)
	return nil
}

// InstallIcon installs the application icon.
func InstallIcon(system bool) error {
	data, err := resources.ReadFile("resources/gguard.png")
	if err != nil {
		return fmt.Errorf("cannot read the embedded icon: %w", err)
	}

	dir := iconDir(system)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("cannot create the icon directory %s: %w", dir, err)
	}

	iconPath := filepath.Join(dir, "gguard.png")
	if err := os.WriteFile(iconPath, data, 0644); err != nil {
		return fmt.Errorf("cannot write the icon: %w", err)
	}

	log.Printf("installed icon: %s", iconPath)
	return nil
}

// RemoveIcon deletes the application icon.
func RemoveIcon(system bool) error {
	path := filepath.Join(iconDir(system), "gguard.png")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	log.Printf("removed icon: %s", path)
	return nil
}

// InstallStartMenuShortcut is a no-op off Windows: the desktop entry covers it.
func InstallStartMenuShortcut(_ string, _ bool) error { return nil }

// RemoveStartMenuShortcut is a no-op off Windows.
func RemoveStartMenuShortcut(_ bool) error { return nil }

// InstallAppsRegistry is a no-op off Windows: there is no Apps & Features.
func InstallAppsRegistry(_ string, _ bool) error { return nil }

// RemoveAppsRegistry is a no-op off Windows.
func RemoveAppsRegistry(_ bool) error { return nil }

// scheduleRemoveOnReboot has no equivalent off Windows, so the leftover goes
// right away — which works because only Windows locks a running executable.
func scheduleRemoveOnReboot(path string) {
	_ = os.Remove(path)
}
