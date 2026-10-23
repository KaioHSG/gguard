//go:build !windows

package engine

import "os/exec"

// shellCommand builds a command that runs a full shell line through sh.
func shellCommand(line string) *exec.Cmd {
	return exec.Command("sh", "-c", line)
}
