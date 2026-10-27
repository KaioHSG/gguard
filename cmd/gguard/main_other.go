//go:build !windows

package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func isServiceManager() bool {
	return false
}

// isTerminal reports whether stdin is attached to a terminal (char device), so
// a bare invocation from a shell prints usage instead of the installer.
func isTerminal() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

func startElevated(args []string) bool {
	exe := filepath.Join(exeDir(), exeName())
	fullArgs := append([]string{exe}, args...)
	return exec.Command("pkexec", fullArgs...).Run() == nil ||
		exec.Command("sudo", fullArgs...).Run() == nil
}

func showInstallDialog() {
	_, installed := detectInstallType()

	fmt.Println("=== GopherGuard Installer ===")
	fmt.Println()

	bufr := bufio.NewReader(os.Stdin)

	if installed {
		fmt.Println("GopherGuard is already installed.")
		fmt.Println("[U] Upgrade")
		fmt.Println("[C] Cancel")
		fmt.Print("Choice: ")

		lineb, _, errl := bufr.ReadLine()
		if errl == io.EOF || errl != nil {
			return
		}
		choice := strings.TrimSpace(string(lineb))

		if strings.HasPrefix(strings.ToLower(choice), "u") {
			doUpgrade()
		}
	} else {
		fmt.Println("GopherGuard is not installed.")
		fmt.Println("[A] Admin install")
		fmt.Println("[U] User install")
		fmt.Println("[C] Cancel")
		fmt.Print("Choice: ")

		lineb, _, errl := bufr.ReadLine()
		if errl == io.EOF || errl != nil {
			return
		}
		choice := strings.TrimSpace(string(lineb))
		lower := strings.ToLower(choice)

		if strings.HasPrefix(lower, "a") {
			if !isAdmin() {
				fmt.Println("Requesting administrator privileges...")
				startElevated([]string{"install"})
				return
			}
			doInstall(false)
		} else if strings.HasPrefix(lower, "u") {
			doInstall(true)
		}
	}

	pauseBeforeExit()
}

func pauseBeforeExit() {
	fmt.Println()
	fmt.Print("Press Enter to close...")
	bufr := bufio.NewReader(os.Stdin)
	bufr.ReadLine()
}

func activeUserConfigDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".local", "bin")
}

func userConfigDir() string {
	return installTargetDir(true)
}
