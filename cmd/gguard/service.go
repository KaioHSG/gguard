package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/KaioHSG/gguard/install"
	"github.com/kardianos/service"
)

type program struct {
	ctx    context.Context
	cancel context.CancelFunc
}

func (p *program) Start(s service.Service) error {
	p.ctx, p.cancel = context.WithCancel(context.Background())
	go runGuards(p.ctx, newGuardSession())
	return nil
}

func (p *program) Stop(s service.Service) error {
	if p.cancel != nil {
		p.cancel()
	}
	return nil
}

// newService builds the service this process runs. Only the service manager
// path uses it, so it is bound to the real program.
func newService(p *program, userService bool) service.Service {
	exeDir := exeDir()

	var args []string
	if guardsFlag != "" {
		args = append(args, "--guards", install.AbsPath(guardsFlag, exeDir))
	}
	for _, s := range scriptsFlag {
		args = append(args, "--scripts", install.AbsPath(s, exeDir))
	}

	name := "gguard"
	displayName := "GopherGuard"
	if userService {
		name = "gguard-user"
		displayName = "GopherGuard (User)"
	}

	s, err := install.NewFor(p, install.Config{
		Name:             name,
		DisplayName:      displayName,
		Description:      "File watcher and backup automator",
		Arguments:        args,
		WorkingDirectory: exeDir,
		UserService:      userService,
	})
	if err != nil {
		log.Fatal(err)
	}
	return s
}

func serviceControl(s service.Service, action string) error {
	if err := install.Control(s, action); err != nil {
		return err
	}

	switch action {
	case "start":
		time.Sleep(2 * time.Second)
		status, err := s.Status()
		if err != nil {
			fmt.Fprintf(os.Stderr, "gguard: service start requested but status check failed: %v\n", err)
			fmt.Fprintln(os.Stderr, "Check gguard.log for details.")
			return nil
		}
		switch status {
		case service.StatusRunning:
			log.Println("gguard service started.")
		case service.StatusStopped:
			logFile := filepath.Join(exeDir(), "gguard.log")
			fmt.Fprintln(os.Stderr, "gguard service failed to start.")
			fmt.Fprintf(os.Stderr, "Check gguard.log for details (%s)\n", logFile)
			if data, err := os.ReadFile(logFile); err == nil {
				lines := strings.Split(string(data), "\n")
				start := len(lines) - 5
				if start < 0 {
					start = 0
				}
				fmt.Fprintln(os.Stderr, "Last log entries:")
				for _, l := range lines[start:] {
					if l != "" {
						fmt.Fprintf(os.Stderr, "  %s\n", l)
					}
				}
			}
		default:
			log.Println("gguard service status unknown.")
		}
	case "stop":
		log.Println("gguard service stopped.")
	case "restart":
		log.Println("gguard service restarted.")
	}

	return nil
}

func statusMessage(status service.Status) string {
	switch status {
	case service.StatusRunning:
		return "running"
	case service.StatusStopped:
		return "stopped"
	default:
		return "unknown"
	}
}

func isServiceMode() bool {
	return isServiceManager()
}

func setupServiceLog(exeDir string) {
	logFile := filepath.Join(exeDir, "gguard.log")
	f, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err == nil {
		log.SetOutput(f)
		log.Printf("gguard: logging to %s", logFile)
	} else {
		log.Printf("gguard: cannot open log file %s: %v", logFile, err)
	}
}
