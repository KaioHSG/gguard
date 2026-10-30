package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"

	"github.com/KaioHSG/gguard/install"
)

// doInstall writes the binaries, registers the service and wires up the
// shortcuts, the PATH entry and the tray setting. Running it over an existing
// installation upgrades it in place.
//
// The installer is not copied into the install directory: uninstalling is the
// app's job (see install.Uninstall), so the directory holds the two binaries
// and nothing else.
func doInstall(userService bool) {
	wasInstalled := false
	wasRunning := false
	if isUser, installed := install.DetectType(); installed {
		wasInstalled = true
		wasRunning, _ = install.RunningAny()
		if isUser != userService {
			log.Fatalf("a %s installation already exists. Use 'gguard-setup install%s' to upgrade it.",
				install.ModeName(isUser), map[bool]string{true: " --user", false: ""}[isUser])
		}
		log.Println("upgrading existing installation...")
	}

	targetDir := install.TargetDir(userService)

	if !install.IsAdmin() && !userService {
		log.Fatal("a system install needs administrator rights. Use --user for a per-user install.")
	}

	if err := installPayload(targetDir); err != nil {
		log.Fatalf("%v", err)
	}

	// Earlier versions kept a copy of the installer next to the binaries so
	// that uninstalling would stay possible. It does not belong there anymore.
	removeStaleSetup(targetDir)

	args := installFiles(targetDir)

	// A per-user install on Windows has no service: Windows needs the account
	// password to register one, and the tray already starts the guards when the
	// user logs on. Elsewhere the per-user mode is a real service
	// (systemd --user, launchd).
	if userService && runtime.GOOS == "windows" {
		log.Println("per-user install on Windows: no service, the tray starts the guards at logon.")
	} else {
		registerService(userService, targetDir, args, wasInstalled, wasRunning)
	}

	system := !userService
	if err := install.InstallDesktopEntry(targetDir, system); err != nil {
		warn("cannot install the desktop entry: %v", err)
	}
	if err := install.InstallIcon(system); err != nil {
		warn("cannot install the icon: %v", err)
	}
	if err := install.InstallStartMenuShortcut(targetDir, system); err != nil {
		warn("cannot install the Start Menu shortcut: %v", err)
	}
	if err := install.InstallAppsRegistry(targetDir, system); err != nil {
		warn("cannot register in Apps & Features: %v", err)
	}
	if err := install.AddToPath(targetDir, system); err != nil {
		warn("cannot update PATH: %v", err)
	}

	// The tray is a setting: turn it on for a fresh install and keep whatever
	// the user chose ('gguard tray --on/--off') when upgrading. Either way the
	// entry is rewritten, so an installation from a version without a launcher
	// starts going through it and stops flashing a console.
	if !wasInstalled || install.TrayEnabled() {
		if err := install.EnableTrayAutostart(install.LauncherPath(filepath.Join(targetDir, install.ExeName()))); err != nil {
			warn("cannot enable tray autostart: %v", err)
		}
	}

	fmt.Printf("\nGopherGuard installed to %s\n", targetDir)
	fmt.Println("Restart your terminal for the PATH change to take effect.")
}

// registerService installs the service for a mode that has one, stopping an
// existing one first and putting it back the way it was found: an upgrade that
// leaves the service stopped would silently stop watching files.
func registerService(userService bool, targetDir string, args []string, wasInstalled, wasRunning bool) {
	name := install.ServiceName(userService)
	displayName := "GopherGuard"
	if userService {
		displayName = "GopherGuard (User)"
	}

	svc, err := install.New(install.Config{
		Name:             name,
		DisplayName:      displayName,
		Description:      "File watcher and backup automator",
		Executable:       filepath.Join(targetDir, install.ExeName()),
		Arguments:        args,
		WorkingDirectory: targetDir,
		UserService:      userService,
	})
	if err != nil {
		log.Fatalf("cannot prepare the service: %v", err)
	}

	if install.ServiceExists(name) {
		log.Printf("stopping the existing %s service...", name)
		if old, err := install.New(install.Config{Name: name, DisplayName: name}); err == nil {
			_ = install.Control(old, "stop")
		}
	}

	if err := install.Control(svc, "install"); err != nil {
		warn("%v", err)
	}

	if wasInstalled {
		if wasRunning {
			if err := install.Control(svc, "start"); err != nil {
				warn("%v", err)
			}
		} else {
			log.Println("the gguard service is stopped. Run 'gguard start' to start it.")
		}
	}
}

// installFiles copies the optional scripts and config next to the binaries and
// returns the service arguments that point at them.
func installFiles(targetDir string) []string {
	srcDir, err := os.Getwd()
	if err != nil {
		srcDir = exeDir()
	}

	var args []string
	hasScripts := false

	copyConfig := func(path, label string) {
		if path == "" {
			return
		}
		if _, err := os.Stat(path); err != nil {
			warn("%s not found: %s", label, path)
			return
		}
		dst := filepath.Join(targetDir, filepath.Base(path))
		if err := install.CopyFile(path, dst); err != nil {
			warn("cannot copy %s: %v", label, err)
			return
		}
		log.Printf("copied %s -> %s", filepath.Base(path), targetDir)
	}

	if guardsFlag != "" {
		guardsPath := install.AbsPath(guardsFlag, srcDir)
		copyConfig(guardsPath, "guards config")
		if _, err := os.Stat(filepath.Join(targetDir, filepath.Base(guardsPath))); err == nil {
			args = append(args, "--guards", filepath.Join(targetDir, filepath.Base(guardsPath)))
		}
	} else if _, err := os.Stat(filepath.Join(srcDir, "guards.json")); err == nil {
		copyConfig(filepath.Join(srcDir, "guards.json"), "guards.json")
		args = append(args, "--guards", filepath.Join(targetDir, "guards.json"))
	}

	sources := scriptsFlag
	if len(sources) == 0 {
		if info, err := os.Stat(filepath.Join(srcDir, "gg-scripts")); err == nil && info.IsDir() {
			sources = append(sources, filepath.Join(srcDir, "gg-scripts"))
		}
	}

	for _, src := range sources {
		path := install.AbsPath(src, srcDir)
		info, err := os.Stat(path)
		if err != nil {
			warn("scripts path not found: %s", path)
			continue
		}

		dst := filepath.Join(targetDir, filepath.Base(path))
		if info.IsDir() {
			if err := install.CopyDir(path, dst); err != nil {
				warn("cannot copy %s: %v", path, err)
				continue
			}
		} else if err := install.CopyFile(path, dst); err != nil {
			warn("cannot copy %s: %v", path, err)
			continue
		}

		args = append(args, "--scripts", dst)
		hasScripts = true
	}

	if !hasScripts {
		scriptsDir := filepath.Join(targetDir, "gg-scripts")
		if entries, err := os.ReadDir(scriptsDir); err != nil || len(entries) == 0 {
			log.Printf("no .ggs scripts yet: put them in %s and they are picked up on their own.", scriptsDir)
		} else {
			log.Printf("nothing to copy from here; keeping the scripts already in %s.", scriptsDir)
		}
	}
	return args
}

// doUninstall removes the installation the mode points at.
func doUninstall(userService bool) {
	if !userFlagExplicit() {
		if isUser, installed := install.DetectType(); !installed {
			fmt.Println("GopherGuard is not installed")
			return
		} else if isUser {
			log.Println("detected a user installation, uninstalling...")
			userService = true
		}
	} else if !install.ServiceExists(install.ServiceName(userService)) {
		fmt.Println(install.ModeHint(userService))
		return
	}

	if err := install.Uninstall(userService); err != nil {
		log.Fatalf("%v", err)
	}
	fmt.Printf("GopherGuard uninstalled from %s\n", install.TargetDir(userService))
}

func doUpgrade() {
	isUser, installed := install.DetectType()
	if !installed {
		doInstall(false)
		return
	}

	log.Printf("upgrading the existing %s installation...", install.ModeName(isUser))
	doInstall(isUser)
}

// removeStaleSetup deletes the installer copy older releases left in the
// install directory. Failing is fine: it may be the file that is running.
func removeStaleSetup(targetDir string) {
	for _, name := range []string{"gguard-setup.exe", "gguard-setup"} {
		path := filepath.Join(targetDir, name)
		if _, err := os.Stat(path); err != nil {
			continue
		}
		if err := os.Remove(path); err != nil {
			warn("cannot remove the old %s: %v", name, err)
			continue
		}
		log.Printf("removed the old %s from the install directory", name)
	}
}

func userFlagExplicit() bool {
	for _, arg := range os.Args {
		if arg == "--user" || arg == "-user" {
			return true
		}
	}
	return false
}

// exeDir is where this installer was started from, used as a fallback source
// for scripts and config.
func exeDir() string {
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(exe)
}
