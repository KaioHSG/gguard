package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/user"
	"path/filepath"
	"time"

	"github.com/kardianos/service"
)

type program struct {
	ctx    context.Context
	cancel context.CancelFunc
}

func (p *program) Start(s service.Service) error {
	p.ctx, p.cancel = context.WithCancel(context.Background())
	go runGuards(p.ctx)
	return nil
}

func (p *program) Stop(s service.Service) error {
	if p.cancel != nil {
		p.cancel()
	}
	return nil
}

func newService(p *program, userService bool) service.Service {
	exeDir := exeDir()

	var args []string
	if guardsFlag != "" {
		args = append(args, "--guards", absPath(guardsFlag, exeDir))
	}
	for _, s := range scriptsFlag {
		args = append(args, "--scripts", absPath(s, exeDir))
	}

	name := "gguard"
	displayName := "GopherGuard"

	if userService {
		name = "gguard-user"
		displayName = "GopherGuard (User)"
	}

	cfg := &serviceConfig{
		Name:             name,
		DisplayName:      displayName,
		Description:      "File watcher and backup automator",
		Arguments:        args,
		WorkingDirectory: exeDir,
		UserService:      userService,
	}

	return buildService(p, cfg)
}

func buildService(p *program, cfg *serviceConfig) service.Service {
	svcCfg := &service.Config{
		Name:             cfg.Name,
		DisplayName:      cfg.DisplayName,
		Description:      cfg.Description,
		Arguments:        cfg.Arguments,
		WorkingDirectory: cfg.WorkingDirectory,
	}

	if cfg.Executable != "" {
		svcCfg.Executable = cfg.Executable
	}

	if cfg.UserService {
		if u, err := user.Current(); err == nil {
			svcCfg.UserName = u.Username
		}
	}

	s, err := service.New(p, svcCfg)
	if err != nil {
		log.Fatal(err)
	}
	return s
}

func serviceControl(s service.Service, action string) {
	err := service.Control(s, action)
	if err != nil {
		log.Fatalf("gguard %s: %v", action, err)
	}

	switch action {
	case "install":
		log.Println("gguard installed as service. Use 'gguard --start' or your OS service manager to start it.")
	case "uninstall":
		log.Println("gguard uninstalled.")
	case "stop":
		log.Println("gguard service stopped.")
	case "restart":
		log.Println("gguard service restarted.")
	case "start":
		time.Sleep(2 * time.Second)
		status, err := s.Status()
		if err != nil {
			log.Println("gguard service start requested (could not verify status)")
			return
		}
		switch status {
		case service.StatusRunning:
			log.Println("gguard service started.")
		case service.StatusStopped:
			log.Println("gguard service failed to start. Check gguard.log for details.")
		default:
			log.Println("gguard service status unknown.")
		}
	}
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

func absPath(p, exeDir string) string {
	if filepath.IsAbs(p) {
		return p
	}
	abs, err := filepath.Abs(filepath.Join(exeDir, p))
	if err != nil {
		return p
	}
	return abs
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

func logServiceStart() {
	fmt.Fprintln(os.Stderr, "gguard: running as a system service - logs in gguard.log")
}

func logServiceStop() {
	log.Println("gguard service: shutting down...")
}