package engine

import (
	"time"

	"github.com/KaioHSG/gguard/ggs"
)

// BuildVars merges the config variables with the guard's own variables and
// resolves {watch}. Shared by New, RunOnce and Describe.
func BuildVars(guard *ggs.Guard, configVars map[string]string) map[string]string {
	vars := make(map[string]string)
	for k, v := range configVars {
		vars[k] = v
	}
	vars["watch"] = resolveVars(guard.Watch, vars)
	return vars
}

// ResolvedCommand is a command with its variables already substituted.
type ResolvedCommand struct {
	Verb string
	Args []string
}

// Describe returns the resolved arguments of every command without executing
// anything. Used by the check command for a dry run.
func Describe(guard *ggs.Guard, configVars map[string]string) []ResolvedCommand {
	vars := BuildVars(guard, configVars)
	vars["timestamp"] = time.Now().Format("2006-01-02_15-04-05")

	disp := NewDispatcher(guard.Name, vars)
	out := make([]ResolvedCommand, 0, len(guard.Commands))
	for _, cmd := range guard.Commands {
		args := make([]string, len(cmd.Args))
		for i, a := range cmd.Args {
			args[i] = disp.resolve(a)
		}
		out = append(out, ResolvedCommand{Verb: cmd.Verb, Args: args})
	}
	return out
}

// RunOnce executes every command of a guard a single time, without starting a
// watcher. onStep, when set, is called with the resolved command before it
// runs. It returns the first error encountered, if any.
func RunOnce(guard *ggs.Guard, configVars map[string]string, onStep func(ResolvedCommand)) error {
	vars := BuildVars(guard, configVars)
	vars["timestamp"] = time.Now().Format("2006-01-02_15-04-05")
	vars["file"] = ""

	disp := NewDispatcher(guard.Name, vars)
	var runErr error
	for _, cmd := range guard.Commands {
		if onStep != nil {
			args := make([]string, len(cmd.Args))
			for i, a := range cmd.Args {
				args[i] = disp.resolve(a)
			}
			onStep(ResolvedCommand{Verb: cmd.Verb, Args: args})
		}
		if err := disp.Execute(cmd); err != nil {
			if runErr == nil {
				runErr = err
			}
		}
	}
	return runErr
}
