//go:build windows

package engine

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

// shellCommand builds a command that runs a full shell line through cmd.exe.
// CmdLine is set directly so quoting (spaces in paths, redirections) survives
// intact, which exec.Command's argument escaping would otherwise mangle.
//
// CREATE_NO_WINDOW keeps the shell from opening a console window: a guard runs
// from the tray or the service, neither of which has a console of its own, so
// cmd.exe would otherwise flash one on every execution.
func shellCommand(line string) *exec.Cmd {
	cmd := exec.Command("cmd")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CmdLine:       "cmd /c " + line,
		CreationFlags: windows.CREATE_NO_WINDOW,
	}
	return cmd
}
