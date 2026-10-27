package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLogRingAccumulatesPartialLines(t *testing.T) {
	ring := newLogRing(10)

	if _, err := ring.Write([]byte("first\nsec")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if got, want := ring.Snapshot(), "first\nsec"; got != want {
		t.Fatalf("Snapshot = %q, want %q", got, want)
	}

	// The remainder of the second line arrives in a later write.
	if _, err := ring.Write([]byte("ond\r\nthird\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if got, want := ring.Snapshot(), "first\nsecond\nthird"; got != want {
		t.Fatalf("Snapshot = %q, want %q", got, want)
	}
}

func TestLogRingKeepsOnlyTheTail(t *testing.T) {
	ring := newLogRing(3)
	for _, line := range []string{"1\n", "2\n", "3\n", "4\n", "5\n"} {
		if _, err := ring.Write([]byte(line)); err != nil {
			t.Fatalf("Write: %v", err)
		}
	}

	if got, want := ring.Snapshot(), "3\n4\n5"; got != want {
		t.Fatalf("Snapshot = %q, want %q", got, want)
	}
}

func TestTailFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gguard.log")
	if err := os.WriteFile(path, []byte("a\nb\nc\nd\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if got, want := tailFile(path, 2), "c\nd"; got != want {
		t.Fatalf("tailFile = %q, want %q", got, want)
	}
	if got, want := tailFile(path, 99), "a\nb\nc\nd"; got != want {
		t.Fatalf("tailFile = %q, want %q", got, want)
	}
	if got := tailFile(filepath.Join(t.TempDir(), "missing.log"), 5); got != "" {
		t.Fatalf("tailFile of a missing file = %q, want \"\"", got)
	}
}

func TestLocalOnlyRejectsForeignHosts(t *testing.T) {
	handler := localOnly(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for _, host := range []string{"127.0.0.1:8642", "localhost", "[::1]:8642"} {
		req := httptest.NewRequest(http.MethodGet, "http://"+host+"/api/state", nil)
		req.Host = host
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("host %q: status = %d, want 200", host, rec.Code)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "http://evil.test/api/state", nil)
	req.Host = "evil.test"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("foreign host: status = %d, want 403", rec.Code)
	}
}

func TestGUIStateReportsGuards(t *testing.T) {
	sess := newGuardSession()
	sess.tracker.Add("Demo", "/tmp/watched")
	sess.tracker.SetResult("Demo", nil)

	g := &gui{
		sess:   sess,
		status: func() (GuardState, string) { return StateOK, "ok" },
		logs:   func() string { return "hello" },
	}

	rec := httptest.NewRecorder()
	g.routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/state", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var state guiState
	if err := json.Unmarshal(rec.Body.Bytes(), &state); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if state.Version != Version {
		t.Errorf("version = %q, want %q", state.Version, Version)
	}
	if state.Overall != "ok" || state.Label != "ok" {
		t.Errorf("overall/label = %q/%q, want \"ok\"/\"ok\"", state.Overall, state.Label)
	}
	if len(state.Guards) != 1 || state.Guards[0].Name != "Demo" {
		t.Fatalf("guards = %+v, want one entry named Demo", state.Guards)
	}
	if state.Log != "hello" {
		t.Errorf("log = %q, want \"hello\"", state.Log)
	}
	if !strings.HasSuffix(state.ScriptsDir, "gg-scripts") {
		t.Errorf("scripts_dir = %q, want a gg-scripts path", state.ScriptsDir)
	}
}

func TestGUIRunRejectsGetAndAcceptsAnyGuard(t *testing.T) {
	sess := newGuardSession()
	g := &gui{
		sess:   sess,
		status: trayStatus(sess),
		logs:   func() string { return "" },
	}
	routes := g.routes()

	rec := httptest.NewRecorder()
	routes.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/run", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET /api/run status = %d, want 405", rec.Code)
	}

	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/run", strings.NewReader(`{"name":"Unknown"}`))
	routes.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("POST /api/run status = %d, want 202", rec.Code)
	}
}

func TestGUIPageServesTheEmbeddedDashboard(t *testing.T) {
	g := &gui{}
	rec := httptest.NewRecorder()
	g.routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "<title>GopherGuard</title>") {
		t.Error("dashboard page does not look like the embedded asset")
	}

	rec = httptest.NewRecorder()
	g.routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/nope", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("GET /nope status = %d, want 404", rec.Code)
	}
}
