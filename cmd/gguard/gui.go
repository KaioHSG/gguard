package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// guiDefaultPort is the dashboard port. When it is taken the OS picks a free
// one instead, so the dashboard never fails to start because of a conflict.
const guiDefaultPort = 8642

// logRingLines is how much process output the dashboard keeps in memory.
const logRingLines = 300

// guiAttached tells runGuards that a dashboard is on screen, so having no
// scripts installed is not fatal (the page still shows the service state).
var guiAttached bool

// guiState is the snapshot the dashboard renders.
type guiState struct {
	Version    string        `json:"version"`
	Service    string        `json:"service"`
	Overall    string        `json:"overall"`
	Label      string        `json:"label"`
	Guards     []statusEntry `json:"guards"`
	Log        string        `json:"log"`
	ScriptsDir string        `json:"scripts_dir"`
}

// gui serves the dashboard over loopback.
type gui struct {
	sess   *guardSession
	status func() (GuardState, string)
	logs   func() string
	// cancel stops the whole process when Quit is used in the page.
	cancel context.CancelFunc
}

// doGUI opens the dashboard in the browser. Guards, the dashboard and — when
// the tray setting is on — the tray icon all run together in this process. If
// another instance is already serving the dashboard (the tray, for example) it
// simply opens it.
func doGUI() {
	if url := probeDashboard(); url != "" {
		fmt.Printf("GopherGuard is already running — opening %s\n", url)
		openURL(url)
		return
	}

	guiAttached = true

	// Started from a launcher (shortcut, double-click) there is no console to
	// print to: drop its window and keep the log in gguard.log instead.
	if !isTerminal() {
		quietFlag = true
		hideOwnConsole()
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runForeground(ctx, cancel, foregroundOptions{dashboard: true, openBrowser: true})
}

// stopEverything ends a foreground run. The page's Quit stops the guards and,
// when a tray icon is showing, closes it too — the same as the tray's Quit.
func stopEverything(cancel context.CancelFunc) context.CancelFunc {
	return func() {
		quitTray()
		cancel()
	}
}

// startDashboard serves the dashboard on loopback and returns its URL.
func startDashboard(sess *guardSession, status func() (GuardState, string), ring *logRing, cancel context.CancelFunc) (string, *http.Server, error) {
	ln, err := listenLocal()
	if err != nil {
		return "", nil, err
	}

	g := &gui{
		sess:   sess,
		status: status,
		logs:   guiLogs(ring),
		cancel: cancel,
	}

	srv := &http.Server{Handler: localOnly(g.routes())}
	go func() {
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Printf("dashboard: %v", err)
		}
	}()

	return "http://" + ln.Addr().String(), srv, nil
}

func closeDashboard(srv *http.Server) {
	if srv != nil {
		_ = srv.Close()
	}
}

// listenLocal binds the dashboard to loopback, preferring its usual port.
func listenLocal() (net.Listener, error) {
	if ln, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(guiDefaultPort)); err == nil {
		return ln, nil
	}
	return net.Listen("tcp", "127.0.0.1:0")
}

// probeDashboard reports the URL of an instance already serving the dashboard,
// or "" when there is none.
func probeDashboard() string {
	client := &http.Client{Timeout: 500 * time.Millisecond}
	url := "http://127.0.0.1:" + strconv.Itoa(guiDefaultPort)

	resp, err := client.Get(url + "/api/state")
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	var state guiState
	if resp.StatusCode != http.StatusOK || json.NewDecoder(resp.Body).Decode(&state) != nil || state.Version == "" {
		return ""
	}
	return url
}

func (g *gui) routes() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/", g.page)
	mux.HandleFunc("/api/state", g.state)
	mux.HandleFunc("/api/run", g.run)
	mux.HandleFunc("/api/quit", g.quit)
	return mux
}

func (g *gui) page(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	data, err := embedded.ReadFile("resources/dashboard.html")
	if err != nil {
		http.Error(w, "dashboard asset missing", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(data)
}

// state is the polling endpoint behind the auto-refresh.
func (g *gui) state(w http.ResponseWriter, r *http.Request) {
	st, label := g.status()

	svc := "not installed"
	if running, known := serviceRunningFor(); known {
		if running {
			svc = "running"
		} else {
			svc = "stopped"
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(guiState{
		Version:    Version,
		Service:    svc,
		Overall:    st.String(),
		Label:      label,
		Guards:     g.sess.tracker.Entries(),
		Log:        g.logs(),
		ScriptsDir: filepath.Join(exeDir(), "gg-scripts"),
	})
}

// run triggers every guard, or a single one when the body carries a name.
func (g *gui) run(w http.ResponseWriter, r *http.Request) {
	if !postOnly(w, r) {
		return
	}

	var req struct {
		Name string `json:"name"`
	}
	_ = json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&req)

	if strings.TrimSpace(req.Name) == "" {
		g.sess.runAll()
	} else {
		g.sess.runOne(req.Name)
	}
	w.WriteHeader(http.StatusAccepted)
}

func (g *gui) quit(w http.ResponseWriter, r *http.Request) {
	if !postOnly(w, r) || g.cancel == nil {
		return
	}

	_, _ = w.Write([]byte("stopping"))
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	go g.cancel()
}

func postOnly(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return false
	}
	return true
}

// localOnly rejects requests whose Host is not loopback, so a random web page
// cannot poke the dashboard through DNS rebinding.
func localOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if h, _, err := net.SplitHostPort(r.Host); err == nil {
			host = h
		}
		switch host {
		case "127.0.0.1", "localhost", "::1":
			next.ServeHTTP(w, r)
		default:
			http.Error(w, "forbidden", http.StatusForbidden)
		}
	})
}

// guiLogs returns the text shown in the dashboard log panel: the in-memory tail
// when there is one, otherwise the tail of gguard.log.
func guiLogs(ring *logRing) func() string {
	logPath := filepath.Join(exeDir(), "gguard.log")
	return func() string {
		if ring != nil {
			if s := ring.Snapshot(); s != "" {
				return s
			}
		}
		return tailFile(logPath, 40)
	}
}

// logTee mirrors the process log so the dashboard can show it even when the
// logger is redirected to gguard.log (quiet mode) or the console is gone.
var logTee *logRing

// teeLogs mirrors the process log into memory for the dashboard. runGuards
// adds the log file to the same output when running in quiet mode.
func teeLogs() *logRing {
	ring := newLogRing(logRingLines)
	logTee = ring
	log.SetOutput(io.MultiWriter(os.Stderr, ring))
	return ring
}

// quitDashboard asks a running instance to stop through its own endpoint, and
// reports whether one answered.
func quitDashboard() bool {
	client := &http.Client{Timeout: 2 * time.Second}
	url := "http://127.0.0.1:" + strconv.Itoa(guiDefaultPort) + "/api/quit"

	resp, err := client.Post(url, "application/json", strings.NewReader("{}"))
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusAccepted
}

// logRing keeps the tail of the process log in memory.
type logRing struct {
	mu    sync.Mutex
	max   int
	lines []string
	part  string
}

func newLogRing(max int) *logRing { return &logRing{max: max} }

func (r *logRing) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.part += string(p)
	for {
		i := strings.IndexByte(r.part, '\n')
		if i < 0 {
			break
		}
		r.append(strings.TrimRight(r.part[:i], "\r"))
		r.part = r.part[i+1:]
	}

	// Guard against a writer that never emits a newline.
	for len(r.part) > 4096 {
		r.append(r.part[:4096])
		r.part = r.part[4096:]
	}
	return len(p), nil
}

func (r *logRing) append(line string) {
	r.lines = append(r.lines, line)
	if len(r.lines) > r.max {
		r.lines = r.lines[len(r.lines)-r.max:]
	}
}

// Snapshot returns the buffered lines, oldest first.
func (r *logRing) Snapshot() string {
	r.mu.Lock()
	defer r.mu.Unlock()

	lines := append([]string{}, r.lines...)
	if strings.TrimSpace(r.part) != "" {
		lines = append(lines, r.part)
	}
	return strings.Join(lines, "\n")
}

// tailFile returns the last n lines of a text file, or "" when it cannot be
// read.
func tailFile(path string, n int) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}

	lines := strings.Split(strings.TrimRight(string(data), "\r\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, "\r")
	}
	return strings.Join(lines, "\n")
}
