//go:build !windows

package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

func installTargetDir(userService bool) string {
	if userService {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, ".local", "bin")
	}
	return "/usr/local/bin"
}

func exeName() string {
	return "gguard"
}

func isAdmin() bool {
	return os.Geteuid() == 0
}

func addToPath(targetDir string, system bool) error {
	log.Printf("gguard installed to %s (ensure it is in your PATH)", targetDir)
	return nil
}

func removeFromPath(targetDir string, system bool) error {
	return nil
}

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

func installDesktopEntry(targetDir string, system bool) error {
	data, err := embedded.ReadFile("resources/gguard.desktop")
	if err != nil {
		return fmt.Errorf("cannot read embedded desktop entry: %w", err)
	}

	exePath := filepath.Join(targetDir, exeName())
	content := strings.ReplaceAll(string(data), "{exec}", exePath)

	dir := desktopDir(system)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("cannot create desktop directory %s: %w", dir, err)
	}

	desktopPath := filepath.Join(dir, "gguard.desktop")
	if err := os.WriteFile(desktopPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("cannot write desktop entry: %w", err)
	}

	log.Printf("installed desktop entry: %s", desktopPath)
	return nil
}

func uninstallDesktopEntry(system bool) error {
	path := filepath.Join(desktopDir(system), "gguard.desktop")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	log.Printf("removed desktop entry: %s", path)
	return nil
}

func installIcon(system bool) error {
	data, err := embedded.ReadFile("resources/gguard.png")
	if err != nil {
		return fmt.Errorf("cannot read embedded icon: %w", err)
	}

	dir := iconDir(system)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("cannot create icon directory %s: %w", dir, err)
	}

	iconPath := filepath.Join(dir, "gguard.png")
	if err := os.WriteFile(iconPath, data, 0644); err != nil {
		return fmt.Errorf("cannot write icon: %w", err)
	}

	log.Printf("installed icon: %s", iconPath)
	return nil
}

func uninstallIcon(system bool) error {
	path := filepath.Join(iconDir(system), "gguard.png")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	log.Printf("removed icon: %s", path)
	return nil
}

func installStartMenuShortcut(targetDir string, system bool) error { return nil }
func uninstallStartMenuShortcut(system bool) error                { return nil }