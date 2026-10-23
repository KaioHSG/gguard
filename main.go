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
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/KaioHSG/gguard/engine"
	"github.com/KaioHSG/gguard/ggs"
	"github.com/kardianos/service"
)

var Version = "0.2.1"

var (
	quietFlag   bool
	userFlag    bool
	scriptsFlag scriptsList
	guardsFlag  string
)

type scriptsList []string

func (s *scriptsList) String() string { return strings.Join(*s, ", ") }
func (s *scriptsList) Set(v string) error {
	*s = append(*s, v)
	return nil
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

func registerFlags(fs *flag.FlagSet) {
	fs.BoolVar(&quietFlag, "quiet", false, "Log to file instead of console")
	fs.BoolVar(&quietFlag, "q", false, "")
	fs.BoolVar(&userFlag, "user", false, "Force user mode (override auto-detect)")
	fs.Var(&scriptsFlag, "scripts", ".ggs file or directory")
	fs.Var(&scriptsFlag, "s", "")
	fs.Var(&scriptsFlag, "script", "")
	fs.StringVar(&guardsFlag, "guards", "", "Path to guards.json (default: next to gguard)")
	fs.StringVar(&guardsFlag, "g", "", "")
}

func init() {
	registerFlags(flag.CommandLine)

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, `gguard - file watcher and backup automator

Usage:
  gguard <command> [flags]

Commands:
  install [--user]     Install as system service (--user for per-user)
  uninstall [--user]   Uninstall the service
  upgrade              Reinstall keeping the same install type
  start                Start the service
  stop                 Stop the service
  restart              Restart the service
  status               Show service status and active guards
  import <file>        Import a .ggs script or guards.json
  export               Export all .ggs scripts and guards.json
  version              Show version
  help                 Show this help

Examples:
  gguard install
  gguard install --user
  gguard import my-script.ggs
  gguard -s my-script.ggs -g guards.json -q

Flags:
  --user               Force user mode (auto-detect: admin=system, user=user)
  -q, --quiet          Log to file instead of console
  -s, --scripts <path> .ggs file or directory
  -g, --guards <path>  Path to guards.json (default: next to gguard)
`)
	}
}

func main() {
	for i, arg := range os.Args {
		if strings.HasPrefix(arg, "--") && len(arg) > 2 && arg[2] != '-' {
			os.Args[i] = "-" + arg[2:]
		}
	}

	flag.Parse()

	remArgs := flag.Args()
	if len(remArgs) > 0 {
		subcmd := remArgs[0]

		sub := flag.NewFlagSet(subcmd, flag.ExitOnError)
		registerFlags(sub)
		sub.Usage = flag.Usage
		_ = sub.Parse(remArgs[1:])

		dispatch(subcmd, sub.Args())
		return
	}

	if isServiceManager() {
		prg := &program{}
		s := newService(prg, targetMode(userFlag) == "user")
		setupServiceLog(exeDir())
		if err := s.Run(); err != nil {
			log.Fatal(err)
		}
		return
	}

	if isTerminal() || hasCLIFlags() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		runGuards(ctx)
		return
	}

	showInstallDialog()
}

func dispatch(subcmd string, subargs []string) {
	switch subcmd {
	case "install":
		doInstall(userFlag)
	case "uninstall":
		doUninstall(userFlag)
	case "upgrade":
		doUpgrade()
	case "start", "stop", "restart":
		prg := &program{}
		s := newService(prg, targetMode(userFlag) == "user")
		if err := serviceControl(s, subcmd); err != nil {
			log.Fatal(err)
		}
	case "status":
		prg := &program{}
		s := newService(prg, targetMode(userFlag) == "user")
		showStatus(s)
	case "import":
		if len(subargs) == 0 {
			log.Fatal("import requires a file path")
		}
		doImport(subargs[0], targetMode(userFlag))
	case "export":
		doExport(targetMode(userFlag))
	case "version":
		fmt.Printf("gguard v%s\n", Version)
	case "help":
		flag.Usage()
	default:
		fmt.Fprintf(os.Stderr, "gguard: unknown command: %s\n", subcmd)
		flag.Usage()
		os.Exit(1)
	}
}

func hasCLIFlags() bool {
	return quietFlag || len(scriptsFlag) > 0 || guardsFlag != ""
}

func runGuards(ctx context.Context) {
	exeDir := exeDir()
	statusPath := filepath.Join(exeDir, "gguard.status.json")

	if !isServiceMode() && quietFlag {
		logFile := filepath.Join(exeDir, "gguard.log")
		f, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err == nil {
			log.SetOutput(f)
		}
	}

	var allScriptDirs []string
	globalConfigPath := resolveGuardsPath(guardsFlag, exeDir)

	if len(scriptsFlag) > 0 {
		allScriptDirs = append(allScriptDirs, scriptsFlag...)
	} else {
		allScriptDirs = append(allScriptDirs, filepath.Join(exeDir, "gg-scripts"))
	}

	userConfigPath := ""
	if isServiceMode() {
		if userDir := activeUserConfigDir(); userDir != "" {
			userScriptsDir := filepath.Join(userDir, "gg-scripts")
			if _, err := os.Stat(userScriptsDir); err == nil {
				allScriptDirs = append(allScriptDirs, userScriptsDir)
				userConfigPath = filepath.Join(userDir, "guards.json")
				log.Printf("gguard: loading user scripts from %s", userScriptsDir)
			}
		}
	}

	scriptPaths := resolveScripts(allScriptDirs)

	if len(scriptsFlag) == 0 {
		scriptPaths = filterAutostart(globalConfigPath, scriptPaths)
	}

	home := userHomeDir()

	type instance struct {
		eng  *engine.Engine
		name string
	}

	var instances []instance
	var statusEntries []statusEntry

	userCfgDir := ""
	if userConfigPath != "" {
		userCfgDir = filepath.Dir(userConfigPath)
	}

	if len(scriptPaths) > 0 {
		ctx2, cancel2 := context.WithCancel(ctx)
		defer cancel2()

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

			cfg := globalConfigPath
			if userCfgDir != "" && strings.HasPrefix(path, userCfgDir) {
				cfg = userConfigPath
			}

			key := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
			vars := loadConfig(cfg, key)
			vars["home"] = home

			eng, err := engine.New(guard, vars)
			if err != nil {
				log.Printf("skipping %q: %v", guard.Name, err)
				continue
			}

			eng.Start(ctx2)
			instances = append(instances, instance{eng: eng, name: guard.Name})
			statusEntries = append(statusEntries, statusEntry{Name: guard.Name, Watch: eng.Watch()})
			log.Printf("gguard: monitoring %q for %q", guard.Watch, guard.Name)
		}

		if len(instances) > 0 {
			writeStatus(statusPath, statusEntries)
			log.Printf("gguard: %d guard(s) running", len(instances))
		}
	}

	if len(instances) == 0 {
		if isServiceMode() {
			log.Println("gguard: no scripts found. Add .ggs files and restart the service.")
		} else {
			log.Fatal("no scripts found. Create a gg-scripts/ directory or use --scripts.")
		}
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-ctx.Done():
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

	os.Remove(statusPath)
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

func showStatus(s service.Service) {
	status, err := s.Status()
	if err != nil {
		fmt.Printf("gguard: %v\n", err)
		return
	}

	info := fmt.Sprintf("gguard is %s", statusMessage(status))

	statusPath := filepath.Join(exeDir(), "gguard.status.json")
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
			log.Printf("warning: scripts path not found: %s", path)
			continue
		}

		if !info.IsDir() {
			scripts = append(scripts, path)
			continue
		}

		entries, err := os.ReadDir(path)
		if err != nil {
			log.Printf("warning: cannot read scripts dir %s: %v", path, err)
			continue
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

func writeStatus(path string, entries []statusEntry) {
	data, _ := json.MarshalIndent(entries, "", "  ")
	os.WriteFile(path, data, 0644)
}

func runtimeOS() string {
	return runtime.GOOS
}

func targetMode(userFlag bool) string {
	if userFlag {
		return "user"
	}
	if isAdmin() {
		return "system"
	}
	return "user"
}

func configDir(mode string) string {
	if mode == "system" {
		return installTargetDir(false)
	}
	return userConfigDir()
}

func doImport(src, mode string) {
	if src == "" {
		log.Fatal("import requires a file path")
	}

	info, err := os.Stat(src)
	if err != nil {
		log.Fatalf("cannot access %s: %v", src, err)
	}
	if info.IsDir() {
		log.Fatal("cannot import a directory")
	}

	target := configDir(mode)
	ext := strings.ToLower(filepath.Ext(src))
	base := filepath.Base(src)

	switch ext {
	case ".ggs":
		scriptsDir := filepath.Join(target, "gg-scripts")
		if err := os.MkdirAll(scriptsDir, 0755); err != nil {
			log.Fatalf("cannot create scripts directory: %v", err)
		}
		dst := filepath.Join(scriptsDir, base)
		if err := copyFile(src, dst); err != nil {
			log.Fatalf("cannot import script: %v", err)
		}
		fmt.Printf("imported %s -> %s\n", base, dst)

	case ".json":
		importedData, err := os.ReadFile(src)
		if err != nil {
			log.Fatalf("cannot read %s: %v", src, err)
		}

		var imported map[string]map[string]interface{}
		if err := json.Unmarshal(importedData, &imported); err != nil {
			log.Fatalf("invalid guards.json format: %v", err)
		}

		configPath := filepath.Join(target, "guards.json")
		var existing map[string]map[string]interface{}

		if data, err := os.ReadFile(configPath); err == nil {
			json.Unmarshal(data, &existing)
		}
		if existing == nil {
			existing = make(map[string]map[string]interface{})
		}

		for k, v := range imported {
			existing[k] = v
		}

		out, _ := json.MarshalIndent(existing, "", "  ")
		if err := os.WriteFile(configPath, out, 0644); err != nil {
			log.Fatalf("cannot write guards.json: %v", err)
		}
		fmt.Printf("merged %d routine(s) into %s\n", len(imported), configPath)

	default:
		log.Fatalf("unsupported file type: %s (use .ggs or .json)", ext)
	}
}

func doExport(mode string) {
	source := configDir(mode)

	scriptsDir := filepath.Join(source, "gg-scripts")
	if info, err := os.Stat(scriptsDir); err != nil || !info.IsDir() {
		log.Fatal("no scripts to export")
	}

	entries, err := os.ReadDir(scriptsDir)
	if err != nil {
		log.Fatalf("cannot read scripts directory: %v", err)
	}

	fmt.Println("Scripts:")
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".ggs") {
			fmt.Printf("  %s\n", filepath.Join(scriptsDir, e.Name()))
		}
	}

	configPath := filepath.Join(source, "guards.json")
	if _, err := os.Stat(configPath); err == nil {
		fmt.Printf("Config: %s\n", configPath)
	}

	fmt.Println()
	fmt.Printf("To export as zip, copy the contents of:\n  %s\n", source)
}
