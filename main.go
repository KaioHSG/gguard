package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gguard/gguard/engine"
	"github.com/gguard/gguard/ggs"
)

const pidFileName = "gguard.pid"

var (
	statusFlag  bool
	stopFlag    bool
	quietFlag   bool
	scriptsFlag scriptsList
	guardsFlag  string
	pidFilePath string
	statusPath  string
)

type scriptsList []string

func (s *scriptsList) String() string { return strings.Join(*s, ", ") }
func (s *scriptsList) Set(v string) error {
	*s = append(*s, v)
	return nil
}

func init() {
	flag.BoolVar(&statusFlag, "status", false, "Show running guard status")
	flag.BoolVar(&stopFlag, "stop", false, "Stop running instance")
	flag.BoolVar(&quietFlag, "quiet", false, "Log to file instead of console")
	flag.BoolVar(&quietFlag, "q", false, "")
	flag.Var(&scriptsFlag, "scripts", ".ggs file or directory")
	flag.Var(&scriptsFlag, "s", "")
	flag.Var(&scriptsFlag, "script", "")
	flag.StringVar(&guardsFlag, "guards", "", "Path to guards.json (default: next to gguard.exe)")
	flag.StringVar(&guardsFlag, "g", "", "")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, `gguard - file watcher and backup automator

  Double-click gguard.exe to auto-start all routines with autostart:true

Usage:
  gguard -s <file.ggs or directory> [-g <guards.json>] [-q]
  gguard --stop
  gguard --status
  gguard --help

Flags:
`)
		flag.PrintDefaults()
	}
}

type statusEntry struct {
	Name  string `json:"name"`
	Watch string `json:"watch"`
}

type guardsRoutine struct {
	Dest            string `json:"dest"`
	Trusted         bool   `json:"trusted"`
	Super           bool   `json:"super"`
	Autostart       bool   `json:"autostart"`
	FreeCache       string `json:"free_cache"`
	KeepBackups     int    `json:"keep_backups"`
	DeleteOlderThan string `json:"delete_older_than"`
}

func main() {
	for i, arg := range os.Args {
		if strings.HasPrefix(arg, "--") && len(arg) > 2 && arg[2] != '-' {
			os.Args[i] = "-" + arg[2:]
		}
	}

	flag.Parse()

	exeDir := exeDir()
	pidFilePath = filepath.Join(exeDir, pidFileName)
	statusPath = filepath.Join(exeDir, "gguard.status.json")

	if stopFlag {
		doStop()
		return
	}

	if statusFlag {
		showStatus()
		return
	}

	lockInstance()
	writePID()
	defer func() {
		os.Remove(pidFilePath)
		os.Remove(statusPath)
	}()

	autoStart := len(scriptsFlag) == 0

	if autoStart {
		scriptsFlag = []string{filepath.Join(exeDir, "gg-scripts")}
	}

	if quietFlag {
		logFile := filepath.Join(exeDir, "gguard.log")
		f, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err == nil {
			log.SetOutput(f)
		}
	}

	configPath := resolveGuardsPath(guardsFlag, exeDir)
	scriptPaths := resolveScripts(scriptsFlag)

	if autoStart {
		scriptPaths = filterAutostart(configPath, scriptPaths)
		if len(scriptPaths) == 0 {
			log.Fatal("no autostart routines found in guards.json")
		}
	} else if len(scriptPaths) == 0 {
		log.Fatal("no .ggs scripts found")
	}

	home := userHomeDir()

	type instance struct {
		eng   *engine.Engine
		name  string
		watch string
	}

	var instances []instance
	var statusEntries []statusEntry
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for _, path := range scriptPaths {
		data, err := os.ReadFile(path)
		if err != nil {
			log.Printf("skipping %s: %v", path, err)
			continue
		}

		guard, err := ggs.Parse(string(data))
		if err != nil {
			log.Printf("skipping %s: parse error: %v", path, err)
			continue
		}

		if guard.OS != "" && guard.OS != runtimeOS() {
			log.Printf("skipping %q: targets OS %q but current OS is %q", guard.Name, guard.OS, runtimeOS())
			continue
		}

		key := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		vars := loadConfig(configPath, key)
		vars["home"] = home

		eng, err := engine.New(guard, vars)
		if err != nil {
			log.Printf("skipping %q: %v", guard.Name, err)
			continue
		}

		eng.Start(ctx)
		instances = append(instances, instance{eng: eng, name: guard.Name, watch: eng.Watch()})
		statusEntries = append(statusEntries, statusEntry{Name: guard.Name, Watch: eng.Watch()})
		log.Printf("gguard: monitoring %q for %q", guard.Watch, guard.Name)
	}

	if len(instances) == 0 {
		log.Fatal("no guards started")
	}

	writeStatus(statusEntries)
	log.Printf("gguard: %d guard(s) running", len(instances))

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-sigCh:
	case <-checkStopFile(filepath.Join(exeDir, "gguard.stop")):
		os.Remove(filepath.Join(exeDir, "gguard.stop"))
	}

	log.Println("gguard: shutting down...")
	var wg sync.WaitGroup
	for _, inst := range instances {
		wg.Add(1)
		go func(inst instance) {
			defer wg.Done()
			log.Printf("gguard: stopping %q...", inst.name)
			inst.eng.Stop()
		}(inst)
	}
	wg.Wait()
	log.Println("gguard: stopped")
}

func checkStopFile(path string) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			if _, err := os.Stat(path); err == nil {
				return
			}
		}
	}()
	return done
}

func filterAutostart(configPath string, allPaths []string) []string {
	config := loadGuardsConfig(configPath)
	if config == nil {
		return nil
	}

	var filtered []string
	for _, path := range allPaths {
		key := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		if r, ok := config[key]; ok && r.Autostart {
			filtered = append(filtered, path)
		}
	}
	return filtered
}

func loadGuardsConfig(path string) map[string]guardsRoutine {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var config map[string]guardsRoutine
	if json.Unmarshal(data, &config) != nil {
		return nil
	}
	return config
}

func doStop() {
	data, err := os.ReadFile(pidFilePath)
	if err != nil {
		fmt.Println("gguard is not running")
		return
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid == 0 {
		fmt.Println("gguard is not running (stale PID file)")
		return
	}

	if !processRunning(pid) {
		fmt.Println("gguard is not running")
		os.Remove(pidFilePath)
		return
	}

	p, err := os.FindProcess(pid)
	if err != nil {
		fmt.Printf("cannot find process %d: %v\n", pid, err)
		return
	}

	if err := p.Kill(); err != nil {
		fmt.Printf("cannot stop process %d: %v\n", pid, err)
		return
	}

	fmt.Printf("gguard stopped (PID %d)\n", pid)
	os.Remove(pidFilePath)
	os.Remove(statusPath)
}

func lockInstance() {
	if _, err := os.Stat(pidFilePath); err == nil {
		data, err := os.ReadFile(pidFilePath)
		if err == nil {
			pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
			if err == nil && pid > 0 {
				if processRunning(pid) {
					log.Fatalf("gguard is already running (PID %d). Use --status.", pid)
				}
			}
		}
		os.Remove(pidFilePath)
	}
}

func writePID() {
	os.WriteFile(pidFilePath, []byte(strconv.Itoa(os.Getpid())), 0644)
}

func writeStatus(entries []statusEntry) {
	data, _ := json.MarshalIndent(entries, "", "  ")
	os.WriteFile(statusPath, data, 0644)
}

func showStatus() {
	data, err := os.ReadFile(pidFilePath)
	if err != nil {
		fmt.Println("gguard is not running")
		return
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid == 0 {
		fmt.Println("gguard is not running (stale PID file)")
		return
	}

	if !processRunning(pid) {
		fmt.Printf("gguard was running (PID %d) but process is gone (stale lock)\n", pid)
		return
	}

	info := fmt.Sprintf("gguard is running (PID %d)", pid)

	statusData, err := os.ReadFile(statusPath)
	if err == nil {
		var entries []statusEntry
		if json.Unmarshal(statusData, &entries) == nil && len(entries) > 0 {
			info += "\n\nActive guards:"
			for _, e := range entries {
				info += fmt.Sprintf("\n  * %s -> %s", e.Name, e.Watch)
			}
		}
	}

	uptime := processUptime(pid)
	if uptime > 0 {
		info += fmt.Sprintf("\nUptime: %s", uptime.Round(time.Second))
	}

	fmt.Println(info)
}

func exeDir() string {
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(exe)
}

func resolveScripts(paths []string) []string {
	var scripts []string
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			log.Fatalf("cannot access %s: %v", path, err)
		}

		if !info.IsDir() {
			scripts = append(scripts, path)
			continue
		}

		entries, err := os.ReadDir(path)
		if err != nil {
			log.Fatalf("cannot read scripts dir %s: %v", path, err)
		}

		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".ggs") {
				scripts = append(scripts, filepath.Join(path, e.Name()))
			}
		}
	}
	return scripts
}

func resolveGuardsPath(explicit string, exeDir string) string {
	if explicit != "" {
		if filepath.IsAbs(explicit) {
			return explicit
		}
		abs, err := filepath.Abs(explicit)
		if err == nil {
			return abs
		}
		return explicit
	}
	return filepath.Join(exeDir, "guards.json")
}

func loadConfig(path string, key string) map[string]string {
	vars := make(map[string]string)

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return vars
		}
		log.Printf("warning: cannot read config %s: %v", path, err)
		return vars
	}

	var config map[string]map[string]interface{}
	if err := json.Unmarshal(data, &config); err != nil {
		log.Printf("warning: invalid config %s: %v", path, err)
		return vars
	}

	routine, ok := config[key]
	if !ok {
		return vars
	}

	for k, v := range routine {
		switch val := v.(type) {
		case string:
			home, _ := os.UserHomeDir()
			val = strings.ReplaceAll(val, "${HOME}", home)
			vars[k] = os.ExpandEnv(val)
		case bool:
			vars[k] = fmt.Sprintf("%t", val)
		case float64:
			vars[k] = fmt.Sprintf("%.0f", val)
		}
	}

	return vars
}

func userHomeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		log.Printf("warning: cannot get home dir: %v", err)
		return ""
	}
	return home
}

func runtimeOS() string {
	return runtime.GOOS
}