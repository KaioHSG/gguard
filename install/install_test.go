package install

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAbsPath(t *testing.T) {
	dir := t.TempDir()

	if got, want := AbsPath("scripts", dir), filepath.Join(dir, "scripts"); got != want {
		t.Errorf("AbsPath(relative) = %q, want %q", got, want)
	}

	abs := filepath.Join(dir, "elsewhere")
	if got := AbsPath(abs, dir); got != abs {
		t.Errorf("AbsPath(absolute) = %q, want %q", got, abs)
	}
}

func TestCopyFileReplacesTheDestination(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")

	if err := os.WriteFile(src, []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, []byte("stale and longer"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := CopyFile(src, dst); err != nil {
		t.Fatalf("CopyFile: %v", err)
	}

	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "original" {
		t.Errorf("destination = %q, want %q", got, "original")
	}
}

func TestCopyDirIsRecursive(t *testing.T) {
	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "out")

	if err := os.MkdirAll(filepath.Join(src, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	for path, body := range map[string]string{
		"top.ggs":         "top",
		"nested/deep.ggs": "deep",
	} {
		if err := os.WriteFile(filepath.Join(src, path), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if err := CopyDir(src, dst); err != nil {
		t.Fatalf("CopyDir: %v", err)
	}

	for path, want := range map[string]string{
		"top.ggs":         "top",
		"nested/deep.ggs": "deep",
	} {
		got, err := os.ReadFile(filepath.Join(dst, path))
		if err != nil {
			t.Fatalf("missing %s: %v", path, err)
		}
		if string(got) != want {
			t.Errorf("%s = %q, want %q", path, got, want)
		}
	}
}
