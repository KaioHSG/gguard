package engine

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/KaioHSG/gguard/ggs"
)

func TestExecuteMessage(t *testing.T) {
	disp := NewDispatcher(map[string]string{})
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

	disp := NewDispatcher(map[string]string{})
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

	disp := NewDispatcher(map[string]string{"var": "test"})
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
	disp := NewDispatcher(map[string]string{})
	err := disp.Execute(ggs.Command{Verb: "nonexistent"})
	if err == nil {
		t.Fatal("expected error for unknown command")
	}
	if !strings.Contains(err.Error(), "unknown command") {
		t.Errorf("expected 'unknown command' error, got: %v", err)
	}
}
