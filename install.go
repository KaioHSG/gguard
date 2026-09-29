package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
)

func detectInstallType() (isUser bool, installed bool) {
	prg := &program{}

	cfgSys := &serviceConfig{Name: "gguard", DisplayName: "GopherGuard"}
	sSys := buildService(prg, cfgSys)
	if _, err := sSys.Status(); err == nil {
		return false, true
	}

	cfgUser := &serviceConfig{Name: "gguard-user", DisplayName: "GopherGuard (User)"}
	sUser := buildService(prg, cfgUser)
	if _, err := sUser.Status(); err == nil {
		return true, true
	}

	return false, false
}

func serviceExists(name string) bool {
	prg := &program{}
	cfg := &serviceConfig{Name: name, DisplayName: name}
	s := buildService(prg, cfg)
	_, err := s.Status()
	return err == nil
}

func doInstall(userService bool) {
	if isUser, installed := detectInstallType(); installed {
		if isUser != userService {
			label := "system"
			if isUser {
				label = "user"
			}
			log.Fatalf("a %s installation already exists. Use --install%s to upgrade it.",
				label, map[bool]string{true: " --user", false: ""}[isUser])
		}
		log.Println("upgrading existing installation...")
	}

	srcDir := exeDir()
	targetDir := installTargetDir(userService)

	if !isAdmin() && !userService {
		log.Fatal("system install requires administrator privileges. Use --user for per-user install.")
	}

	if err := os.MkdirAll(targetDir, 0755); err != nil {
		log.Fatalf("cannot create install directory %s: %v", targetDir, err)
	}

	exeSrc := filepath.Join(srcDir, exeName())
	exeDst := filepath.Join(targetDir, exeName())
	if err := copyFile(exeSrc, exeDst); err != nil {
		log.Fatalf("cannot copy binary to %s: %v", exeDst, err)
	}
	log.Printf("copied %s -> %s", exeName(), targetDir)

	var args []string
	hasGuards := false
	hasScripts := false

	if guardsFlag != "" {
		guardsSrc := absPath(guardsFlag, srcDir)
		if _, err := os.Stat(guardsSrc); err == nil {
			guardsName := filepath.Base(guardsSrc)
			guardsDst := filepath.Join(targetDir, guardsName)
			if err := copyFile(guardsSrc, guardsDst); err != nil {
				log.Printf("warning: cannot copy guards config: %v", err)
			} else {
				log.Printf("copied %s -> %s", guardsName, targetDir)
				args = append(args, "--guards", guardsDst)
				hasGuards = true
			}
		} else {
			log.Printf("warning: guards config not found: %s", guardsSrc)
		}
	} else {
		guardsSrc := filepath.Join(srcDir, "guards.json")
		if _, err := os.Stat(guardsSrc); err == nil {
			guardsDst := filepath.Join(targetDir, "guards.json")
			if err := copyFile(guardsSrc, guardsDst); err != nil {
				log.Printf("warning: cannot copy guards.json: %v", err)
			} else {
				args = append(args, "--guards", guardsDst)
				hasGuards = true
			}
		}
	}

	if len(scriptsFlag) > 0 {
		for _, s := range scriptsFlag {
			src := absPath(s, srcDir)
			name := filepath.Base(src)
			dst := filepath.Join(targetDir, name)
			if info, err := os.Stat(src); err == nil {
				if info.IsDir() {
					if err := copyDir(src, dst); err != nil {
						log.Printf("warning: cannot copy %s: %v", name, err)
					} else {
						args = append(args, "--scripts", dst)
						hasScripts = true
					}
				} else {
					if err := copyFile(src, dst); err != nil {
						log.Printf("warning: cannot copy %s: %v", name, err)
					} else {
						args = append(args, "--scripts", dst)
						hasScripts = true
					}
				}
			} else {
				log.Printf("warning: scripts path not found: %s", src)
			}
		}
	} else {
		scriptsSrc := filepath.Join(srcDir, "gg-scripts")
		scriptsDst := filepath.Join(targetDir, "gg-scripts")
		if info, err := os.Stat(scriptsSrc); err == nil && info.IsDir() {
			if err := copyDir(scriptsSrc, scriptsDst); err != nil {
				log.Printf("warning: cannot copy gg-scripts: %v", err)
			} else {
				args = append(args, "--scripts", scriptsDst)
				hasScripts = true
			}
		}
	}

	if !hasScripts {
		log.Fatal("no scripts found. Create a gg-scripts/ directory or use --scripts to specify scripts.")
	}
	_ = hasGuards

	name := "gguard"
	displayName := "GopherGuard"
	if userService {
		name = "gguard-user"
		displayName = "GopherGuard (User)"
	}

	prg := &program{}
	cfg := &serviceConfig{
		Name:             name,
		DisplayName:      displayName,
		Description:      "File watcher and backup automator",
		Executable:       exeDst,
		Arguments:        args,
		WorkingDirectory: targetDir,
		UserService:      userService,
	}

	s := buildService(prg, cfg)
	serviceControl(s, "install")

	if err := installDesktopEntry(targetDir, !userService); err != nil {
		log.Printf("warning: cannot install desktop entry: %v", err)
	}
	if err := installIcon(!userService); err != nil {
		log.Printf("warning: cannot install icon: %v", err)
	}
	if err := installStartMenuShortcut(targetDir, !userService); err != nil {
		log.Printf("warning: cannot install start menu shortcut: %v", err)
	}

	if err := addToPath(targetDir, !userService); err != nil {
		log.Printf("warning: cannot update PATH: %v", err)
	}

	fmt.Printf("\ngguard installed to %s\n", targetDir)
	fmt.Println("You may need to restart your terminal for PATH changes to take effect.")
}

func doUninstall(userService bool) {
	if !userFlagExplicit() {
		isUser, installed := detectInstallType()
		if !installed {
			fmt.Println("gguard is not installed")
			return
		}
		if isUser && !userService {
			log.Println("detected user installation, uninstalling...")
			userService = true
		}
	} else {
		name := "gguard"
		if userService {
			name = "gguard-user"
		}
		if !serviceExists(name) {
			fmt.Printf("gguard %s service is not installed\n", map[bool]string{true: "user", false: "system"}[userService])
			return
		}
	}

	targetDir := installTargetDir(userService)

	name := "gguard"
	if userService {
		name = "gguard-user"
	}

	prg := &program{}
	cfg := &serviceConfig{
		Name:        name,
		DisplayName: name,
	}

	s := buildService(prg, cfg)
	serviceControl(s, "uninstall")

	if err := uninstallDesktopEntry(!userService); err != nil {
		log.Printf("warning: cannot remove desktop entry: %v", err)
	}
	if err := uninstallIcon(!userService); err != nil {
		log.Printf("warning: cannot remove icon: %v", err)
	}
	if err := uninstallStartMenuShortcut(!userService); err != nil {
		log.Printf("warning: cannot remove start menu shortcut: %v", err)
	}

	if err := removeFromPath(targetDir, !userService); err != nil {
		log.Printf("warning: cannot update PATH: %v", err)
	}

	fmt.Printf("gguard uninstalled from %s\n", targetDir)
}

func doUpgrade() {
	isUser, installed := detectInstallType()
	if !installed {
		doInstall(false)
		return
	}

	label := "system"
	if isUser {
		label = "user"
	}
	log.Printf("upgrading existing %s installation...", label)
	doInstall(isUser)
}

func userFlagExplicit() bool {
	for _, arg := range os.Args {
		if arg == "--user" || arg == "-user" {
			return true
		}
	}
	return false
}

func copyFile(src, dst string) error {
	s, err := os.Open(src)
	if err != nil {
		return err
	}
	defer s.Close()

	d, err := os.Create(dst)
	if err != nil {
		return err
	}

	if _, err := io.Copy(d, s); err != nil {
		d.Close()
		return err
	}
	return d.Close()
}

func copyDir(src, dst string) error {
	if err := os.MkdirAll(dst, 0755); err != nil {
		return err
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, e := range entries {
		srcPath := filepath.Join(src, e.Name())
		dstPath := filepath.Join(dst, e.Name())

		if e.IsDir() {
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			if err := copyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}
	return nil
}

type serviceConfig struct {
	Name             string
	DisplayName      string
	Description      string
	Executable       string
	Arguments        []string
	WorkingDirectory string
	UserService      bool
}