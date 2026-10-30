package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/KaioHSG/gguard/engine"
	"github.com/KaioHSG/gguard/install"
	"github.com/KaioHSG/gguard/ggs"
	"github.com/kardianos/service"
)

var (
	quietFlag   bool
	userFlag    bool
	followFlag  bool
	trayOnFlag         bool
	trayOffFlag        bool
	trayQuitFlag       bool
	trayForegroundFlag bool
	versionFlag bool
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
	loaded  chan struct{}
	once    sync.Once
	// reload carries requests to re-read the routines on disk, stop carries
	// requests to stop them without ending the process. Signalling keeps the
	// tray and the dashboard from having to touch the engines themselves:
	// runGuards owns them.
	reload chan struct{}
	stop   chan struct{}
	// paused records that the routines were stopped on purpose, so the status
	// can say so and an automatic reload does not quietly bring them back.
	paused bool
}

func newGuardSession() *guardSession {
	return &guardSession{
		tracker: NewStatusTracker(),
		loaded:  make(chan struct{}),
		reload:  make(chan struct{}, 1),
		stop:    make(chan struct{}, 1),
	}
}

// requestReload asks runGuards to re-read the routines. It never blocks: a
// pending request is enough, since a second one would do the same work.
func (s *guardSession) requestReload() {
	select {
	case s.reload <- struct{}{}:
	default:
	}
}

// requestStop asks runGuards to stop the routines and keep the process — the
// tray and the dashboard stay up, which is what makes it different from
// quitting.
func (s *guardSession) requestStop() {
	select {
	case s.stop <- struct{}{}:
	default:
	}
}

// setPaused records whether the routines are stopped on purpose.
func (s *guardSession) setPaused(paused bool) {
	s.mu.Lock()
	s.paused = paused
	s.mu.Unlock()
}

// Paused reports whether the routines are stopped on purpose.
func (s *guardSession) Paused() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.paused
}

// reset stops every engine and forgets them, so a reload can start from a
// clean slate.
func (s *guardSession) reset() {
	s.stopAll()
	s.mu.Lock()
	s.engines = nil
	s.mu.Unlock()
}

// markLoaded signals that the initial script scan is done, so the tray can
// build its menu with the full guard list.
func (s *guardSession) markLoaded() {
	s.once.Do(func() { close(s.loaded) })
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

// runOne triggers a single guard by name.
func (s *guardSession) runOne(name string) {
	s.mu.Lock()
	var target *engine.Engine
	for _, e := range s.engines {
		if e.Name() == name {
			target = e
			break
		}
	}
	s.mu.Unlock()

	if target != nil {
		go target.RunNow()
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

// runRunner runs the routines and serves the dashboard: the process that has to
// stay alive for backups to happen. The tray icon is a separate process and
// only a view of this one, so closing it never stops anything.
func runRunner(ctx context.Context, cancel context.CancelFunc, openBrowser bool) {
	if !acquireSingleton(runnerPidPath()) {
		fatalUserError("GopherGuard is already running (use 'gguard stop' to stop it)")
	}
	defer releaseSingleton(runnerPidPath())
	defer clearRunnerURL()

	sess := newGuardSession()
	ring := teeLogs()

	done := make(chan struct{})
	go func() {
		runGuards(ctx, sess)
		close(done)
	}()

	// Wait for the initial scan so the dashboard lists every routine.
	<-sess.loaded

	url, srv, err := startDashboard(sess, trayStatus(sess), ring, cancel)
	if err != nil {
		if openBrowser {
			fatalUserError("cannot start the dashboard: %v", err)
		}
		log.Printf("dashboard: %v", err)
	} else {
		defer closeDashboard(srv)
		writeRunnerURL(url)
	}

	if url != "" && openBrowser {
		fmt.Printf("GopherGuard dashboard: %s\n", url)
		openURL(url)
	}

	<-done
}

// trayStatus reports the overall state shown by the tray: the aggregate guard
// state when guards are loaded, otherwise the current service state.
func trayStatus(sess *guardSession) func() (GuardState, string) {
	return func() (GuardState, string) {
		if sess.Paused() {
			return StateIdle, "routines stopped"
		}
		if sess.count() > 0 {
			st := sess.tracker.Aggregate()
			return st, st.String()
		}

		running, known := serviceRunningFor()
		switch {
		case known && running:
			return StateOK, "service running"
		case known && !running:
			return StateIdle, "service stopped"
		default:
			return StateIdle, "no guards"
		}
	}
}

// serviceRunningFor reports whether any installed gguard service exists and is
// running, using minimal access so it works without administrator rights.
func serviceRunningFor() (running bool, known bool) {
	if r, ok := serviceRunning("gguard"); ok {
		return r, true
	}
	if r, ok := serviceRunning("gguard-user"); ok {
		return r, true
	}
	return false, false
}

// doTrayCommand handles the tray subcommand, which is about the icon:
//
//	gguard tray                 show the icon (starting the runner if needed)
//	gguard tray --quit          close the icon; the routines keep running
//	gguard tray --on            the setting: the icon at logon, which brings
//	                            the runner with it
//	gguard tray --off           the setting: nothing at logon, and stop what
//	                            is running now
//	gguard tray --foreground    be the icon here instead of detaching (what the
//	                            logon entry spawns)
//
// Starting, stopping and the setting are three separate things: a configuration
// flag is not how a process gets bounced, and closing the icon is never how the
// backups stop.
func doTrayCommand(on, off, quit, foreground bool, args []string) {
	if len(args) > 0 {
		log.Fatalf("gguard tray takes flags, not %q: use --on, --off or --quit", args[0])
	}

	if n := countTrue(on, off, quit, foreground); n > 1 {
		log.Fatal("choose one of --on, --off, --quit or --foreground")
	}

	// --foreground runs the icon, which only exists where there is a tray.
	if foreground || (!on && !off && !quit) {
		if !trayAvailable() {
			log.Fatal("the system tray is not supported on this platform")
		}
	}

	switch {
	case foreground:
		if !acquireSingleton(iconPidPath()) {
			log.Fatal("a tray is already running (use 'gguard tray --quit' to close it)")
		}
		defer releaseSingleton(iconPidPath())

		ensureRunner()
		runTray(trayOptions{
			runnerURL: runnerURL,
			logPath:   filepath.Join(exeDir(), "gguard.log"),
			configDir: exeDir(),
			start:     func() { startRunner(false) },
		})
	case quit:
		// Just the icon: the settings and the routines are untouched.
		stopIcon()
	case off:
		if err := install.DisableTrayAutostart(); err != nil {
			log.Printf("warning: cannot disable tray autostart: %v", err)
		}
		stopIcon()
		stopRunner()
		fmt.Println("GopherGuard will not start at logon anymore")
	case on:
		exe, err := os.Executable()
		if err != nil {
			log.Fatalf("cannot locate gguard: %v", err)
		}
		// The windowless launcher keeps the logon entry from flashing a console.
		if err := install.EnableTrayAutostart(install.LauncherPath(exe)); err != nil {
			log.Printf("warning: cannot enable tray autostart: %v", err)
			return
		}
		startIcon()
		fmt.Println("GopherGuard starts at logon from now on")
	default:
		startIcon()
	}
}

// doStartStopRestart drives the runner, for installations without a service.
func doStartStopRestart(action string) {
	switch action {
	case "start":
		startRunner(trayForegroundFlag)
	case "stop":
		stopRunner()
	case "restart":
		stopRunner()
		startRunner(trayForegroundFlag)
	}
}

// showRunnerStatus reports an installation whose routines are run by the runner
// process: whether it is up, and what each routine last did.
func showRunnerStatus() {
	url := runnerURL()
	if url == "" {
		fmt.Println("GopherGuard is not running. Start it with 'gguard start'.")
	} else if pid, ok := livePid(runnerPidPath()); ok {
		fmt.Printf("GopherGuard is running (pid %d) at %s\n", pid, url)
	} else {
		fmt.Printf("GopherGuard is running at %s (a service installs owns it)\n", url)
	}
	printGuards(filepath.Join(exeDir(), "gguard.status.json"))
}

func countTrue(flags ...bool) int {
	n := 0
	for _, f := range flags {
		if f {
			n++
		}
	}
	return n
}

// doReload asks a running GopherGuard to re-read its routines. The request is a
// sentinel file rather than a signal, because a service has no way to be
// reached in memory.
func doReload() {
	if !anyGuardProcessRunning() {
		fmt.Println("no running GopherGuard to reload")
		return
	}

	path := filepath.Join(exeDir(), "gguard.reload")
	if err := os.WriteFile(path, []byte("reload"), 0644); err != nil {
		userService, _ := resolveService()
		elevationHint(userService)
		log.Fatalf("cannot request a reload: %v", err)
	}
	fmt.Println("reload requested; the routines are re-read within a couple of seconds")
}

// anyGuardProcessRunning reports whether a runner or a service is running here,
// so a reload request has somewhere to land.
func anyGuardProcessRunning() bool {
	if _, ok := runnerRunning(); ok {
		return true
	}
	return anyServiceRunning()
}

// reloadCheckInterval is how often a running GopherGuard looks for reload
// requests and for changes to the routines on disk.
const reloadCheckInterval = 2 * time.Second

// GopherGuard foreground work is split between two processes that live and die
// independently:
//
//	the runner  owns the routines and the dashboard: it is what has to be alive
//	            for backups to happen;
//	the icon    is a view of the runner, in its own process, so closing it never
//	            stops a backup.
//
// Both keep a pid file in the user's cache directory (a system-wide install
// directory is not writable, and a pid file that cannot be written would defeat
// the singleton).
func processPidPath(kind string) string {
	name := "gguard." + kind + ".pid"

	dir, err := os.UserCacheDir()
	if err != nil {
		return filepath.Join(exeDir(), name)
	}
	dir = filepath.Join(dir, "GopherGuard")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return filepath.Join(exeDir(), name)
	}
	return filepath.Join(dir, name)
}

func runnerPidPath() string { return processPidPath("runner") }
func iconPidPath() string   { return processPidPath("icon") }

func readPid(path string) (int, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 0 {
		return 0, false
	}
	return pid, true
}

// livePid reports the pid in a pid file when that process is still alive.
func livePid(path string) (int, bool) {
	pid, ok := readPid(path)
	if !ok || !processAlive(pid) {
		return 0, false
	}
	return pid, true
}

// acquireSingleton claims a pid file for this process, so two runners (or two
// icons) never stack up.
func acquireSingleton(path string) bool {
	if pid, ok := livePid(path); ok && pid != os.Getpid() {
		return false
	}
	_ = os.WriteFile(path, []byte(strconv.Itoa(os.Getpid())), 0644)
	return true
}

func releaseSingleton(path string) {
	if pid, ok := readPid(path); ok && pid == os.Getpid() {
		os.Remove(path)
	}
}

// runnerURLPath is where the runner publishes its dashboard address, so the
// icon and the CLI do not have to guess a port.
func runnerURLPath() string {
	return strings.TrimSuffix(runnerPidPath(), ".pid") + ".url"
}

func writeRunnerURL(url string) { _ = os.WriteFile(runnerURLPath(), []byte(url), 0644) }

func clearRunnerURL() { os.Remove(runnerURLPath()) }

// dashboardAnswers reports whether a GopherGuard dashboard is serving at url.
func dashboardAnswers(url string) bool {
	client := &http.Client{Timeout: 500 * time.Millisecond}
	resp, err := client.Get(url + "/api/state")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false
	}

	var state guiState
	return json.NewDecoder(resp.Body).Decode(&state) == nil && state.Version != ""
}

// runnerURL is where the routines are being served right now, or "" when
// nothing answers: the address the runner wrote, otherwise the default port.
func runnerURL() string {
	if data, err := os.ReadFile(runnerURLPath()); err == nil {
		if url := strings.TrimSpace(string(data)); url != "" {
			return url
		}
	}
	url := "http://127.0.0.1:" + strconv.Itoa(guiDefaultPort)
	if dashboardAnswers(url) {
		return url
	}
	return ""
}

// runnerRunning reports the process running the routines, and whether there is
// one at all.
func runnerRunning() (int, bool) {
	if url := runnerURL(); url != "" {
		if pid, ok := livePid(runnerPidPath()); ok {
			return pid, true
		}
		return 0, true
	}
	return 0, false
}

// serviceRunning reports whether a service already runs the routines, in which
// case nothing else should.
func anyServiceRunning() bool {
	if running, known := serviceRunningFor(); known {
		return running
	}
	if running, known := install.RunningAny(); known {
		return running
	}
	return false
}

// ensureRunner starts the runner when nothing else is running the routines.
func ensureRunner() {
	if _, ok := runnerRunning(); ok {
		return
	}
	if anyServiceRunning() {
		return
	}
	startRunner(false)
}

// startRunner starts the process that runs the routines. With foreground it
// *is* that process, which is what the detached spawn uses and what a terminal
// can use to watch it.
func startRunner(foreground bool) {
	if foreground {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		runRunner(ctx, cancel, false)
		return
	}

	if pid, ok := runnerRunning(); ok {
		fmt.Printf("GopherGuard is already running (pid %d)\n", pid)
		return
	}

	exe, err := os.Executable()
	if err != nil {
		log.Fatalf("cannot locate gguard: %v", err)
	}
	if _, err := spawnDetached(exe, []string{"start", "--foreground", "-q"}); err != nil {
		log.Fatalf("cannot start GopherGuard: %v", err)
	}

	// Give the child a moment to claim the singleton so we can report its pid.
	for i := 0; i < 25; i++ {
		time.Sleep(100 * time.Millisecond)
		if pid, ok := runnerRunning(); ok {
			fmt.Printf("GopherGuard started (pid %d)\n", pid)
			return
		}
	}
	fmt.Println("GopherGuard started")
}

// stopRunner stops the process running the routines: through its own dashboard
// first, so the guards stop cleanly, the status file is cleared and the log
// records it. Killing it is only the fallback, for a runner that cannot answer.
func stopRunner() {
	pid, alive := runnerRunning()

	if quitDashboard() {
		for i := 0; i < 20 && alive && processAlive(pid); i++ {
			time.Sleep(100 * time.Millisecond)
		}
		if !alive || !processAlive(pid) {
			os.Remove(runnerPidPath())
			clearRunnerURL()
			fmt.Println("GopherGuard stopped")
			return
		}
	}

	if !alive {
		clearRunnerURL()
		fmt.Println("GopherGuard is not running")
		return
	}

	if err := killProcess(pid); err != nil {
		log.Printf("warning: cannot stop GopherGuard: %v", err)
	}
	os.Remove(runnerPidPath())
	clearRunnerURL()
	fmt.Printf("GopherGuard stopped (pid %d)\n", pid)
}

// startIcon shows the tray icon in its own process, making sure the runner is
// up first: an icon with nothing behind it would just be a dead picture.
func startIcon() {
	if pid, ok := livePid(iconPidPath()); ok {
		fmt.Printf("tray is already on (pid %d)\n", pid)
		return
	}
	ensureRunner()

	exe, err := os.Executable()
	if err != nil {
		log.Fatalf("cannot locate gguard: %v", err)
	}
	// --foreground makes the child the icon itself instead of asking for
	// another one to be started.
	if _, err := spawnDetached(exe, []string{"tray", "--foreground", "-q"}); err != nil {
		log.Fatalf("cannot start the tray: %v", err)
	}

	for i := 0; i < 25; i++ {
		time.Sleep(100 * time.Millisecond)
		if pid, ok := livePid(iconPidPath()); ok {
			fmt.Printf("tray on (pid %d)\n", pid)
			return
		}
	}
	fmt.Println("tray on")
}

// stopIcon closes the tray icon and leaves the runner — and the backups —
// alone.
func stopIcon() {
	pid, ok := livePid(iconPidPath())
	if !ok {
		os.Remove(iconPidPath())
		fmt.Println("the tray is already off")
		return
	}
	if err := killProcess(pid); err != nil {
		log.Printf("warning: cannot stop the tray: %v", err)
	}
	os.Remove(iconPidPath())
	fmt.Printf("tray off (pid %d, GopherGuard keeps running)\n", pid)
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
	fs.BoolVar(&trayOnFlag, "on", false, "Tray: run it now and at logon")
	fs.BoolVar(&trayOffFlag, "off", false, "Tray: stop it and keep it out of logon")
	fs.BoolVar(&trayQuitFlag, "quit", false, "Tray: stop the tray that is running")
	fs.BoolVar(&trayForegroundFlag, "foreground", false, "Tray: be the tray here instead of detaching (the logon entry uses it)")
	fs.BoolVar(&versionFlag, "version", false, "Show version and exit")
	fs.BoolVar(&versionFlag, "v", false, "")
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

Installing and upgrading are handled by gguard-setup; uninstalling lives here,
because the app is what the install directory and Apps & Features point at.

start, stop, restart and status act on whoever runs the routines: the service
when the installation has one, otherwise the runner process (per-user installs
on Windows cannot have a service).

Commands:
  start                Start the routines
  stop                 Stop the routines
  restart              Restart the routines
  status               What is running and what each routine last did
  tray                 Show the tray icon (see --on, --off and --quit)
  reload               Re-read the routines (also picked up on their own)
  gui                  Dashboard in your browser (starts the runner if needed)
  logs [-f]            Show the log file (--follow to keep it open)
  list                 List installed guards
  check <file>         Validate a .ggs script and show what it would do
  run <file>           Run a .ggs script once, in the foreground
  import <file>        Import a .ggs script or guards.json
  export               Export all .ggs scripts and guards.json
  uninstall            Remove GopherGuard, its service and its shortcuts
  version              Show version (same as -v / --version)
  help                 Show this help

Examples:
  gguard status
  gguard import my-script.ggs
  gguard -s my-script.ggs -g guards.json -q

Flags:
  -v, --version        Show version and exit
  --user               Act on the per-user installation (without it, the
                       installed one is detected; it never creates one)
  --on                 Tray: show the icon now and at logon (the setting)
  --off                Tray: stop everything now and keep it out of logon
  --quit               Tray: close the icon only; the routines keep running
  --foreground         Run in this terminal instead of detaching (start, tray)
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

	if versionFlag {
		fmt.Printf("gguard v%s\n", install.Version)
		return
	}

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
		// Running specific scripts with flags is the "watch it happen in this
		// terminal" mode: the routines run here, with the dashboard.
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		runRunner(ctx, cancel, false)
		return
	}

	// With no console there is nowhere to print usage, which means gguard was
	// started by a launcher (double-click, shortcut, logon entry): open the
	// dashboard instead.
	if !consoleAvailable() {
		doGUI()
		return
	}

	flag.Usage()
}

func dispatch(subcmd string, subargs []string) {
	switch subcmd {
	case "install", "upgrade":
		fatalUserError("%s is handled by the installer. Run 'gguard-setup %s'.", subcmd, subcmd)
	case "uninstall":
		userService, installed := resolveService()
		if !installed {
			fmt.Println(install.ModeHint(userService))
			return
		}
		if err := install.Uninstall(userService); err != nil {
			elevationHint(userService)
			log.Fatal(err)
		}
		fmt.Printf("GopherGuard uninstalled from %s\n", install.TargetDir(userService))
	case "start", "stop", "restart":
		userService, installed := resolveService()
		if !installed {
			fatalUserError("%s", install.ModeHint(userService))
		}

		// The commands act on whoever runs the routines: the service when the
		// installation has one, otherwise the runner process — which is what
		// a per-user install on Windows uses, since it cannot have a service.
		if !install.HasService(userService) {
			doStartStopRestart(subcmd)
			return
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
		if !installed {
			fmt.Println(install.ModeHint(userService))
			return
		}
		if !install.HasService(userService) {
			showRunnerStatus()
			return
		}
		prg := &program{}
		s := newService(prg, userService)
		if err := showStatus(s); err != nil {
			elevationHint(userService)
		}
	case "tray":
		doTrayCommand(trayOnFlag, trayOffFlag, trayQuitFlag, trayForegroundFlag, subargs)
	case "reload":
		doReload()
	case "gui":
		doGUI()
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
		fmt.Printf("gguard v%s\n", install.Version)
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

	// Quiet mode, or no console at all (started by the launcher, the logon
	// entry, the shortcut): either way the log belongs in gguard.log.
	if !isServiceMode() && (quietFlag || !consoleAvailable()) {
		logFile := filepath.Join(exeDir, "gguard.log")
		f, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err == nil {
			// The dashboard keeps reading the log even when the install
			// directory is not writable (a system install) and the file
			// cannot be opened.
			if logTee != nil {
				log.SetOutput(io.MultiWriter(f, logTee))
			} else {
				log.SetOutput(f)
			}
		}
	}

	src := resolveSources(exeDir)

	ctx2, cancel2 := context.WithCancel(ctx)
	defer cancel2()

	if n := loadGuards(ctx2, sess, src); n == 0 {
		switch {
		case isServiceMode():
			log.Println("gguard: no scripts found. Add .ggs files and they are picked up automatically.")
		case trayAvailable() || guiAttached:
			log.Println("gguard: no scripts found. Showing service status only.")
		default:
			log.Fatal("no scripts found. Create a gg-scripts/ directory or use --scripts.")
		}
	}

	sess.markLoaded()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Routines are re-read when someone asks for it (the tray, the dashboard,
	// 'gguard reload') or when the files on disk change. No restart needed.
	stopPath := filepath.Join(exeDir, "gguard.stop")
	reloadPath := filepath.Join(exeDir, "gguard.reload")
	sig := configSignature(src)

	ticker := time.NewTicker(reloadCheckInterval)
	defer ticker.Stop()

	for {
		reason := ""
		select {
		case <-ctx.Done():
			shutdown(sess, statusPath)
			return
		case <-sigCh:
			shutdown(sess, statusPath)
			return
		case <-sess.reload:
			reason = "requested"
		case <-sess.stop:
			sess.reset()
			sess.setPaused(true)
			// Nothing is being watched now, and the status file is how a
			// separate 'gguard status' can tell: an empty list means stopped,
			// not "running and idle".
			writeStatus(statusPath, nil)
			sig = configSignature(src)
			log.Println("gguard: routines stopped; the process keeps running")
			continue
		case <-ticker.C:
			if _, err := os.Stat(stopPath); err == nil {
				os.Remove(stopPath)
				shutdown(sess, statusPath)
				return
			}
			current := configSignature(src)
			switch {
			case reloadRequested(reloadPath):
				reason = "requested by 'gguard reload'"
			case sess.Paused():
				// Stopped on purpose: changes on disk are noted, but they do
				// not bring the routines back on their own.
				sig = current
				continue
			case current != sig:
				reason = "routines changed on disk"
			default:
				continue
			}
		}

		log.Printf("gguard: reloading routines (%s)...", reason)
		sess.reset()
		loadGuards(ctx2, sess, src)
		sess.setPaused(false)
		sig = configSignature(src)
	}
}

// reloadRequested consumes the sentinel file asking for a reload.
func reloadRequested(path string) bool {
	if _, err := os.Stat(path); err != nil {
		return false
	}
	os.Remove(path)
	return true
}

// shutdown stops the guards and clears the status file.
func shutdown(sess *guardSession, statusPath string) {
	log.Println("gguard: shutting down...")
	sess.stopAll()
	log.Println("gguard: stopped")
	os.Remove(statusPath)
}

// routineSources is where routines and their configuration come from.
type routineSources struct {
	scriptDirs []string
	globalCfg  string
	userCfg    string
	userCfgDir string
}

// resolveSources decides which directories hold .ggs scripts — the --scripts
// ones or gg-scripts next to the binary — plus the config of the user logged in
// at the console when running as a system service.
func resolveSources(exeDir string) routineSources {
	src := routineSources{globalCfg: resolveGuardsPath(guardsFlag, exeDir)}

	if len(scriptsFlag) > 0 {
		src.scriptDirs = append(src.scriptDirs, scriptsFlag...)
	} else {
		src.scriptDirs = append(src.scriptDirs, filepath.Join(exeDir, "gg-scripts"))
	}

	if isServiceMode() {
		if userDir := activeUserConfigDir(); userDir != "" {
			userScriptsDir := filepath.Join(userDir, "gg-scripts")
			if _, err := os.Stat(userScriptsDir); err == nil {
				src.scriptDirs = append(src.scriptDirs, userScriptsDir)
				src.userCfg = filepath.Join(userDir, "guards.json")
				src.userCfgDir = userDir
				log.Printf("gguard: loading user scripts from %s", userScriptsDir)
			}
		}
	}
	return src
}

// configFor returns the guards.json that applies to a script: each one uses the
// config from its own directory, so global scripts take the global config and
// user scripts take the active user's.
func (src routineSources) configFor(path string) string {
	if src.userCfgDir != "" && strings.HasPrefix(path, src.userCfgDir) {
		return src.userCfg
	}
	return src.globalCfg
}

// configs lists the config files a reload has to watch.
func (src routineSources) configs() []string {
	out := []string{src.globalCfg}
	if src.userCfg != "" {
		out = append(out, src.userCfg)
	}
	return out
}

// configSignature fingerprints everything a reload depends on: the .ggs files in
// each script directory and the guards.json files. Comparing signatures is how
// a change on disk is noticed without watching every event.
func configSignature(src routineSources) string {
	var b strings.Builder

	for _, dir := range src.scriptDirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			b.WriteString(dir + "|missing\n")
			continue
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".ggs") {
				continue
			}
			info, err := e.Info()
			if err != nil {
				continue
			}
			fmt.Fprintf(&b, "%s|%s|%d|%d\n", dir, e.Name(), info.Size(), info.ModTime().UnixNano())
		}
	}

	for _, cfg := range src.configs() {
		if info, err := os.Stat(cfg); err == nil {
			fmt.Fprintf(&b, "%s|%d|%d\n", cfg, info.Size(), info.ModTime().UnixNano())
		}
	}
	return b.String()
}

// loadGuards reads the routines and starts them, returning how many run. It is
// called at startup and again on every reload; guards that survive keep their
// recorded state and run count.
func loadGuards(ctx context.Context, sess *guardSession, src routineSources) int {
	tracker := sess.tracker
	scriptPaths := resolveScripts(src.scriptDirs)

	if len(scriptsFlag) == 0 {
		scriptPaths = filterAutostart(scriptPaths, src.configFor)
	}

	home := userHomeDir()
	var loaded []string

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

		key := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		vars := loadConfig(src.configFor(path), key)
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

		eng.Start(ctx)
		sess.add(eng)
		loaded = append(loaded, guard.Name)
		log.Printf("gguard: monitoring %q for %q", guard.Watch, guard.Name)
	}

	// Guards whose scripts are gone leave the list with them.
	tracker.Prune(loaded)

	if n := len(loaded); n > 0 {
		log.Printf("gguard: %d guard(s) running", n)
	}
	return len(loaded)
}

func filterAutostart(allPaths []string, configFor func(string) string) []string {
	var filtered []string
	for _, path := range allPaths {
		config := loadGuardsConfig(configFor(path))
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
	var statusErr error
	if status, err := s.Status(); err != nil {
		statusErr = err
		fmt.Printf("gguard service: %v\n", err)
	} else {
		fmt.Printf("gguard service is %s\n", statusMessage(status))
	}

	printGuards(filepath.Join(exeDir(), "gguard.status.json"))
	return statusErr
}

// printGuards lists what each routine last did, from the status file the
// process running them keeps up to date. An empty list means they were stopped
// on purpose, which looks the same as never having run — the caller has just
// said whether anything is running.
func printGuards(statusPath string) {
	statusData, err := os.ReadFile(statusPath)
	if err != nil {
		return
	}

	var entries []statusEntry
	if json.Unmarshal(statusData, &entries) != nil {
		return
	}
	if len(entries) == 0 {
		fmt.Println("\nNo routines are running here.")
		return
	}

	fmt.Println("\nGuards:")
	for _, e := range entries {
		state := e.State
		if state == "" {
			state = "idle"
		}
		fmt.Printf("  [%s] %s -> %s", state, e.Name, e.Watch)
		if e.Runs > 0 {
			fmt.Printf(" (%d run(s))", e.Runs)
		}
		fmt.Println()
		if e.LastError != "" {
			fmt.Printf("      last error: %s\n", e.LastError)
		}
	}
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
	if install.IsAdmin() {
		return "system"
	}
	return "user"
}

// resolveService decides which service a control command should act on. With an
// explicit --user flag it trusts the flag; otherwise it detects what is actually
// installed, so 'gguard start' works regardless of the current privileges.
func resolveService() (userService bool, installed bool) {
	if userFlagExplicit() {
		return userFlag, install.InstalledMode(userFlag)
	}
	return install.DetectType()
}

// userFlagExplicit reports whether --user was typed, which makes the mode an
// explicit choice instead of something to detect.
func userFlagExplicit() bool {
	for _, arg := range os.Args {
		if arg == "--user" || arg == "-user" {
			return true
		}
	}
	return false
}

// elevationHint explains how to run a system-wide (global) service command with
// the privileges it needs. It stays quiet for user installs or when already
// elevated.
func elevationHint(userService bool) {
	if userService || install.IsAdmin() {
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
		return install.TargetDir(false)
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
		if err := install.CopyFile(src, dst); err != nil {
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
