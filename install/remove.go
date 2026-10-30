package install

import (
	"log"
	"os"
	"path/filepath"
)

// ServiceName is the service that runs GopherGuard in the given mode.
func ServiceName(userService bool) string {
	if userService {
		return "gguard-user"
	}
	return "gguard"
}

// ModeName names the mode for messages: "system" or "user".
func ModeName(userService bool) string {
	if userService {
		return "user"
	}
	return "system"
}

// Uninstall removes the service and everything the installer put on the system
// for that mode, then empties the install directory. The executable that is
// running from it always survives — on Windows it is scheduled for removal on
// the next boot, since a running file cannot delete itself.
func Uninstall(userService bool) error {
	name := ServiceName(userService)

	if ServiceExists(name) {
		svc, err := New(Config{Name: name, DisplayName: name})
		if err == nil {
			log.Printf("stopping the %s service...", name)
			_ = Control(svc, "stop")
			if err := Control(svc, "uninstall"); err != nil {
				return err
			}
		}
	}

	targetDir := TargetDir(userService)
	system := !userService

	// Everything below is cleanup: report it, but keep going so a single
	// failure cannot leave the machine half uninstalled.
	if err := RemoveDesktopEntry(system); err != nil {
		warn("cannot remove the desktop entry: %v", err)
	}
	if err := RemoveIcon(system); err != nil {
		warn("cannot remove the icon: %v", err)
	}
	if err := RemoveStartMenuShortcut(system); err != nil {
		warn("cannot remove the Start Menu shortcut: %v", err)
	}
	if err := RemoveAppsRegistry(system); err != nil {
		warn("cannot remove the Apps & Features entry: %v", err)
	}
	if err := RemoveFromPath(targetDir, system); err != nil {
		warn("cannot update PATH: %v", err)
	}
	if err := DisableTrayAutostart(); err != nil {
		warn("cannot remove the tray autostart: %v", err)
	}

	removeDir(targetDir)
	return nil
}

func warn(format string, args ...any) {
	log.Printf("warning: "+format, args...)
}

// removeDir deletes the contents of dir and then dir itself, leaving anything
// Windows keeps locked to the next reboot.
func removeDir(dir string) {
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	leftover := false
	for _, e := range entries {
		path := filepath.Join(dir, e.Name())
		if e.IsDir() {
			if err := os.RemoveAll(path); err != nil {
				log.Printf("warning: could not remove %s: %v", path, err)
			}
			continue
		}
		if err := os.Remove(path); err != nil {
			// Almost always the executable that is running from here.
			log.Printf("warning: could not remove %s: %v", path, err)
			scheduleRemoveOnReboot(path)
			leftover = true
		}
	}

	if leftover {
		scheduleRemoveOnReboot(dir)
		log.Printf("%s will be removed on the next restart", dir)
		return
	}

	if err := os.Remove(dir); err != nil {
		log.Printf("warning: could not remove the install directory %s: %v", dir, err)
	}
}
