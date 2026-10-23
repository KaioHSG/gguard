package engine

import (
	"archive/zip"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/KaioHSG/gguard/ggs"
	"github.com/KaioHSG/gguard/notify"
)

func TestExecuteMessage(t *testing.T) {
	notify.SetEnabled(false)
	defer notify.SetEnabled(true)

	disp := NewDispatcher("test", map[string]string{})
	err := disp.Execute(ggs.Command{
		Verb: "message",
		Args: []string{"hello world"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExecuteZip(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "gguard-test-*")
	if err != nil {
		t.Fatalf("cannot create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	srcDir := filepath.Join(tmpDir, "source")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatalf("cannot create source dir: %v", err)
	}

	testFile := filepath.Join(srcDir, "save.dat")
	if err := os.WriteFile(testFile, []byte("game save data"), 0644); err != nil {
		t.Fatalf("cannot write test file: %v", err)
	}

	zipDest := filepath.Join(tmpDir, "backup.zip")

	disp := NewDispatcher("test", map[string]string{})
	err = disp.Execute(ggs.Command{Verb: "zip", Args: []string{zipDest, srcDir}})
	if err != nil {
		t.Fatalf("zip failed: %v", err)
	}

	if _, err := os.Stat(zipDest); os.IsNotExist(err) {
		t.Fatal("zip file was not created")
	}

	zr, err := zip.OpenReader(zipDest)
	if err != nil {
		t.Fatalf("cannot open zip: %v", err)
	}
	defer zr.Close()

	found := false
	for _, f := range zr.File {
		if f.Name == "save.dat" {
			found = true
			break
		}
	}
	if !found {
		t.Error("save.dat not found in zip")
	}
}

func TestExecuteZipVariableResolution(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "gguard-test-*")
	if err != nil {
		t.Fatalf("cannot create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	srcDir := filepath.Join(tmpDir, "source")
	os.MkdirAll(srcDir, 0755)
	os.WriteFile(filepath.Join(srcDir, "save.dat"), []byte("data"), 0644)

	zipDest := filepath.Join(tmpDir, "backup-{var}.zip")

	disp := NewDispatcher("test", map[string]string{"var": "test"})
	err = disp.Execute(ggs.Command{Verb: "zip", Args: []string{zipDest, srcDir}})
	if err != nil {
		t.Fatalf("zip failed: %v", err)
	}

	expectedZip := filepath.Join(tmpDir, "backup-test.zip")
	if _, err := os.Stat(expectedZip); os.IsNotExist(err) {
		t.Fatalf("expected zip at %s, but it does not exist", expectedZip)
	}
}

func TestDispatcherUnknownCommand(t *testing.T) {
	disp := NewDispatcher("test", map[string]string{})
	err := disp.Execute(ggs.Command{Verb: "nonexistent"})
	if err == nil {
		t.Fatal("expected error for unknown command")
	}
	if !strings.Contains(err.Error(), "unknown command") {
		t.Errorf("expected 'unknown command' error, got: %v", err)
	}
}

func TestRunRequiresTrusted(t *testing.T) {
	disp := NewDispatcher("test", map[string]string{})
	if err := disp.Execute(ggs.Command{Verb: "run", Args: []string{"echo", "hi"}}); err == nil {
		t.Fatal("expected error for run without trusted")
	}
}

func TestRunExecutes(t *testing.T) {
	out := filepath.Join(t.TempDir(), "out.txt")

	cmdline := "echo hi > '" + out + "'"
	if runtime.GOOS == "windows" {
		cmdline = `echo hi > "` + out + `"`
	}

	disp := NewDispatcher("test", map[string]string{"trusted": "true"})
	if err := disp.Execute(ggs.Command{Verb: "run", Args: []string{cmdline}}); err != nil {
		t.Fatalf("run failed: %v", err)
	}

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("command output not written: %v", err)
	}
	if !strings.Contains(string(data), "hi") {
		t.Errorf("unexpected output: %q", data)
	}
}

func TestDeleteRequiresTrusted(t *testing.T) {
	disp := NewDispatcher("test", map[string]string{})
	if err := disp.Execute(ggs.Command{Verb: "delete", Args: []string{"whatever"}}); err == nil {
		t.Fatal("expected error for delete without trusted")
	}
}

func TestDeleteRemovesFile(t *testing.T) {
	f := filepath.Join(t.TempDir(), "gone.txt")
	if err := os.WriteFile(f, []byte("x"), 0644); err != nil {
		t.Fatalf("cannot create file: %v", err)
	}

	disp := NewDispatcher("test", map[string]string{"trusted": "true"})
	if err := disp.Execute(ggs.Command{Verb: "delete", Args: []string{f}}); err != nil {
		t.Fatalf("delete failed: %v", err)
	}
	if _, err := os.Stat(f); !os.IsNotExist(err) {
		t.Error("file was not deleted")
	}
}

func TestWebhookPostsBody(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got = string(b)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	disp := NewDispatcher("test", map[string]string{})
	if err := disp.Execute(ggs.Command{Verb: "webhook", Args: []string{srv.URL, "hello"}}); err != nil {
		t.Fatalf("webhook failed: %v", err)
	}
	if got != "hello" {
		t.Errorf("body = %q, want %q", got, "hello")
	}
}

func TestWebhookServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	disp := NewDispatcher("test", map[string]string{})
	if err := disp.Execute(ggs.Command{Verb: "webhook", Args: []string{srv.URL}}); err == nil {
		t.Fatal("expected error when the server returns 500")
	}
}
