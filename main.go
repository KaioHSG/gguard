package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
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
	followFlag  bool
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
	Name      string `json:"name"`
	Watch     string `json:"watch"`
	State     string `json:"state"`
	Runs      int    `json:"runs"`
	LastError string `json:"last_error,omitempty"`
}

// guardSession owns the running engines so actions like "run now" can reach
// them, and exposes the shared status tracker.
type guardSession struct {
	tracker *StatusTracker
	mu      sync.Mutex
	engines []*engine.Engine
}

func newGuardSession() *guardSession {
	return &guardSession{tracker: NewStatusTracker()}
}

func (s *guardSession) add(e *engine.Engine) {
	s.mu.Lock()
	s.engines = append(s.engines, e)
	s.mu.Unlock()
}

// runAll triggers every guard immediately (tray "Run now").
func (s *guardSession) runAll() {
	s.mu.Lock()
	engines := make([]*engine.Engine, len(s.engines))
	copy(engines, s.engines)
	s.mu.Unlock()

	for _, e := range engines {
		go e.RunNow()
	}
}

func (s *guardSession) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.engines)
}

func (s *guardSession) stopAll() {
	s.mu.Lock()
	engines := make([]*engine.Engine, len(s.engines))
	copy(engines, s.engines)
	s.mu.Unlock()

	var wg sync.WaitGroup
	for _, e := range engines {
		wg.Add(1)
		go func(e *engine.Engine) {
			defer wg.Done()
			e.Stop()
		}(e)
	}
	wg.Wait()
}

// runForeground runs the guards and, on platforms with a tray, keeps a tray
// icon alive on the main goroutine until the user quits it.
func runForeground(ctx context.Context, cancel context.CancelFunc) {
	sess := newGuardSession()

	if !trayAvailable() {
		runGuards(ctx, sess)
		return
	}

	done := make(chan struct{})
	go func() {
		runGuards(ctx, sess)
		close(done)
	}()

	runTray(trayOptions{
		tracker:   sess.tracker,
		runAll:    sess.runAll,
		logPath:   filepath.Join(exeDir(), "gguard.log"),
		configDir: exeDir(),
		onQuit:    cancel,
	})

	<-done
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
	fs.BoolVar(&followFlag, "follow", false, "Follow log output (logs command)")
	fs.BoolVar(&followFlag, "f", false, "")
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
  gguard                       Show this help

Commands:
  install [--user]     Install as system service (--user for per-user)
  uninstall [--user]   Uninstall the service
  upgrade              Reinstall keeping the same install type
  start                Start the service
  stop                 Stop the service
  restart              Restart the service
  status               Show service status and active guards
  tray                 Run in the foreground with a system tray icon
  logs [-f]            Show the log file (--follow to keep it open)
  list                 List installed guards
  check <file>         Validate a .ggs script and show what it would do
  run <file>           Run a .ggs script once, in the foreground
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

	if hasCLIFlags() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		runForeground(ctx, cancel)
		return
	}

	if isTerminal() {
		flag.Usage()
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
		userService, installed := resolveService()
		if !installed {
			log.Fatal("gguard is not installed. Run 'gguard install' first.")
		}

		prg := &program{}
		s := newService(prg, userService)

		if subcmd == "start" {
			if st, err := s.Status(); err == nil && st == service.StatusRunning {
				log.Println("gguard service is already running.")
				return
			}
		}

		if err := serviceControl(s, subcmd); err != nil {
			elevationHint(userService)
			log.Fatal(err)
		}
	case "status":
		userService, installed := resolveService()
		prg := &program{}
		s := newService(prg, userService)
		if err := showStatus(s); err != nil && installed {
			elevationHint(userService)
		}
	case "tray":
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		runForeground(ctx, cancel)
	case "logs":
		doLogs(followFlag)
	case "import":
		if len(subargs) == 0 {
			log.Fatal("import requires a file path")
		}
		doImport(subargs[0], targetMode(userFlag))
	case "list":
		doList(targetMode(userFlag))
	case "check":
		if len(subargs) == 0 {
			log.Fatal("check requires a file path")
		}
		doCheck(subargs[0], targetMode(userFlag))
	case "run":
		if len(subargs) == 0 {
			log.Fatal("run requires a file path")
		}
		doRun(subargs[0], targetMode(userFlag))
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

func runGuards(ctx context.Context, sess *guardSession) {
	exeDir := exeDir()
	statusPath := filepath.Join(exeDir, "gguard.status.json")

	tracker := sess.tracker
	tracker.AddListener(func() {
		writeStatus(statusPath, tracker.Entries())
	})

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

			if !guard.MatchesOS(runtimeOS()) {
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

			eng.SetObserver(func(name string, running bool, err error) {
				if running {
					tracker.SetRunning(name)
					return
				}
				tracker.SetResult(name, err)
			})
			tracker.Add(guard.Name, eng.Watch())

			eng.Start(ctx2)
			sess.add(eng)
			log.Printf("gguard: monitoring %q for %q", guard.Watch, guard.Name)
		}

		if n := sess.count(); n > 0 {
			log.Printf("gguard: %d guard(s) running", n)
		}
	}

	if sess.count() == 0 {
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
	sess.stopAll()
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

func showStatus(s service.Service) error {
	var info string
	var statusErr error
	if status, err := s.Status(); err != nil {
		statusErr = err
		info = fmt.Sprintf("gguard service: %v", err)
	} else {
		info = fmt.Sprintf("gguard service is %s", statusMessage(status))
	}

	statusPath := filepath.Join(exeDir(), "gguard.status.json")
	if statusData, err := os.ReadFile(statusPath); err == nil {
		var entries []statusEntry
		if json.Unmarshal(statusData, &entries) == nil && len(entries) > 0 {
			info += "\n\nGuards:"
			for _, e := range entries {
				state := e.State
				if state == "" {
					state = "idle"
				}
				info += fmt.Sprintf("\n  [%s] %s -> %s", state, e.Name, e.Watch)
				if e.Runs > 0 {
					info += fmt.Sprintf(" (%d run(s))", e.Runs)
				}
				if e.LastError != "" {
					info += fmt.Sprintf("\n      last error: %s", e.LastError)
				}
			}
		}
	}

	fmt.Println(info)
	return statusErr
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

// resolveService decides which service a control command should act on. With an
// explicit --user flag it trusts the flag; otherwise it detects what is actually
// installed, so 'gguard start' works regardless of the current privileges.
func resolveService() (userService bool, installed bool) {
	if userFlagExplicit() {
		name := "gguard"
		if userFlag {
			name = "gguard-user"
		}
		return userFlag, serviceExists(name)
	}
	return detectInstallType()
}

// elevationHint explains how to run a system-wide (global) service command with
// the privileges it needs. It stays quiet for user installs or when already
// elevated.
func elevationHint(userService bool) {
	if userService || isAdmin() {
		return
	}
	if runtimeOS() == "windows" {
		log.Println("hint: GopherGuard is installed system-wide; re-run this command as administrator (from an elevated terminal)")
		return
	}
	log.Println("hint: GopherGuard is installed system-wide; re-run this command with sudo")
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

// configForScript prefers a guards.json next to the script (handy while
// developing a guard) and falls back to the mode's install directory.
func configForScript(file, mode string) string {
	local := filepath.Join(filepath.Dir(file), "guards.json")
	if _, err := os.Stat(local); err == nil {
		return local
	}
	return filepath.Join(configDir(mode), "guards.json")
}

// loadGuardFile parses a .ggs file and resolves its variables from the matching
// guards.json entry.
func loadGuardFile(file, mode string) (*ggs.Guard, map[string]string, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, nil, err
	}
	guard, err := ggs.Parse(string(data))
	if err != nil {
		return nil, nil, err
	}

	key := strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
	vars := loadConfig(configForScript(file, mode), key)
	vars["home"] = userHomeDir()
	return guard, vars, nil
}

func doList(mode string) {
	scriptsDir := filepath.Join(configDir(mode), "gg-scripts")
	paths := resolveScripts([]string{scriptsDir})
	if len(paths) == 0 {
		fmt.Printf("no guards found in %s\n", scriptsDir)
		return
	}

	config := loadGuardsConfig(filepath.Join(configDir(mode), "guards.json"))

	fmt.Printf("%-24s %-9s %s\n", "GUARD", "AUTOSTART", "WATCH")
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		guard, err := ggs.Parse(string(data))
		if err != nil {
			fmt.Printf("%-24s %-9s %s\n", filepath.Base(path), "-", "invalid: "+err.Error())
			continue
		}

		autostart := "no"
		key := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		if r, ok := config[key]; ok && r.Autostart {
			autostart = "yes"
		}

		fmt.Printf("%-24s %-9s %s\n", guard.Name, autostart, guard.Watch)
	}
}

func doCheck(file, mode string) {
	guard, vars, err := loadGuardFile(file, mode)
	if err != nil {
		log.Fatalf("cannot check %s: %v", file, err)
	}

	fmt.Printf("guard:    %s\n", guard.Name)
	fmt.Printf("version:  %s\n", orDash(guard.Version))
	fmt.Printf("os:       %s\n", orDash(guard.OS))
	fmt.Printf("debounce: %s\n", guard.Debounce)

	if !guard.MatchesOS(runtimeOS()) {
		fmt.Printf("\nwarning: script targets %q but this machine is %q\n", guard.OS, runtimeOS())
	}

	watch := engine.BuildVars(guard, vars)["watch"]
	if _, err := os.Stat(watch); err != nil {
		fmt.Printf("warning: watch path does not exist: %s\n", watch)
	}

	fmt.Println("\nPlan:")
	for _, c := range engine.Describe(guard, vars) {
		fmt.Printf("  %s %s\n", c.Verb, strings.Join(c.Args, " "))
	}
	fmt.Println("\nOK")
}

func doRun(file, mode string) {
	guard, vars, err := loadGuardFile(file, mode)
	if err != nil {
		log.Fatalf("cannot run %s: %v", file, err)
	}

	fmt.Printf("running %q...\n", guard.Name)
	runErr := engine.RunOnce(guard, vars, func(cmd engine.ResolvedCommand) {
		fmt.Printf("  > %s %s\n", cmd.Verb, strings.Join(cmd.Args, " "))
	})
	if runErr != nil {
		log.Fatalf("run failed: %v", runErr)
	}
	fmt.Println("done.")
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// doLogs prints the log file and, when follow is set, keeps polling for new
// lines until interrupted.
func doLogs(follow bool) {
	path := filepath.Join(exeDir(), "gguard.log")

	f, err := os.Open(path)
	if err != nil {
		fmt.Printf("no log file at %s\n", path)
		return
	}
	defer f.Close()

	reader := bufio.NewReader(f)
	for {
		line, err := reader.ReadString('\n')
		if line != "" {
			fmt.Print(line)
		}
		if err == nil {
			continue
		}
		if err != io.EOF {
			return
		}
		if !follow {
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
}
