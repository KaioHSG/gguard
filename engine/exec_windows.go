//go:build windows

package engine

import (
	"os/exec"
	"syscall"
)

// shellCommand builds a command that runs a full shell line through cmd.exe.
// CmdLine is set directly so quoting (spaces in paths, redirections) survives
// intact, which exec.Command's argument escaping would otherwise mangle.
func shellCommand(line string) *exec.Cmd {
	cmd := exec.Command("cmd")
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: "cmd /c " + line}
	return cmd
}
