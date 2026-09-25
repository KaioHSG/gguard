package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

func main() {
	customPath := ""
	var passArgs []string

	for i := 1; i < len(os.Args); i++ {
		arg := os.Args[i]
		if (arg == "-p" || arg == "--path") && i+1 < len(os.Args) {
			customPath = os.Args[i+1]
			i++
			continue
		}
		passArgs = append(passArgs, arg)
	}

	cli := findCLI(customPath)

	cmd := exec.Command(cli, passArgs...)

	if len(passArgs) == 0 {
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		cmd.Start()
		return
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		os.Exit(cmd.ProcessState.ExitCode())
	}
}

func findCLI(customPath string) string {
	if customPath != "" {
		return customPath
	}

	exe, _ := os.Executable()
	local := filepath.Join(filepath.Dir(exe), "gguard.exe")
	if _, err := os.Stat(local); err == nil {
		return local
	}

	if p, err := exec.LookPath("gguard.exe"); err == nil {
		return p
	}

	return local
}
