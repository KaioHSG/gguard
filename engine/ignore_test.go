package engine

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIgnorePatterns(t *testing.T) {
	cases := []struct {
		name    string
		pattern string
		path    string
		isDir   bool
		want    bool
	}{
		// A bare name matches at any depth, file or directory.
		{"bare dir", "dist", "dist", true, true},
		{"bare dir nested", "dist", "cmd/dist", true, true},
		{"bare file", "dist", "dist", false, true},
		{"bare name inside", "dist", "cmd/dist/x.go", false, true},
		{"unrelated name", "dist", "cmd/main.go", false, false},

		// A trailing slash asks for a directory.
		{"dir only, directory", "dist/", "cmd/dist", true, true},
		{"dir only, file", "dist/", "dist", false, false},

		// A leading slash anchors to the root.
		{"anchored root", "/dist", "dist", true, true},
		{"anchored does not reach deeper", "/dist", "cmd/dist", true, false},

		// A slash inside also anchors.
		{"path anchored", "cmd/gguard", "cmd/gguard", true, true},
		{"path anchored deeper", "cmd/gguard", "x/cmd/gguard", true, false},

		// Globs.
		{"extension anywhere", "*.exe", "gguard.exe", false, true},
		{"extension nested", "*.exe", "cmd/gguard/gguard.exe", false, true},
		{"extension is not a directory", "*.exe", "cmd/gguard", true, false},
		{"question mark", "gguard-?.exe", "gguard-a.exe", false, true},
		{"question mark too long", "gguard-?.exe", "gguard-ab.exe", false, false},
		{"star does not cross slashes", "cmd/*/tmp", "cmd/sub/tmp", true, true},
		{"star stops at a slash", "cmd/*.go", "cmd/sub/x.go", false, false},
		{"matched dir covers its contents", "cmd/*", "cmd/sub/tmp", false, true},

		// Doublestar.
		{"doublestar middle", "cmd/**/tmp", "cmd/a/b/tmp", true, true},
		// git's "a/**/b" also matches "a/b": the doublestar spans zero dirs too.
		{"doublestar middle short", "cmd/**/tmp", "cmd/tmp", true, true},
		{"doublestar tail", "logs/**", "logs/a/b.txt", false, true},
		{"doublestar tail dir", "logs/**/", "logs/a/b", true, true},

		// Matching a directory covers everything inside it, so callers that do
		// not walk into skipped directories still see the right answer.
		{"inside a matched dir", "dist", "dist/gguard.exe", false, true},
		{"inside a matched dir deep", ".git", ".git/objects/ab/cdef", false, true},
		{"inside a dir-only match", "dist/", "dist/x/y.exe", false, true},

		// Comments, blanks and escaped leading characters add nothing.
		{"comment", "# dist", "dist", true, false},
		{"blank", "   ", "dist", true, false},

		// The root itself is never matched.
		{"root", "**", ".", true, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ig := NewIgnore()
			ig.Add(tc.pattern)
			if got := ig.Match(tc.path, tc.isDir); got != tc.want {
				t.Errorf("pattern %q against %q (dir=%v) = %v, want %v",
					tc.pattern, tc.path, tc.isDir, got, tc.want)
			}
		})
	}
}

func TestIgnoreLaterPatternsWin(t *testing.T) {
	ig := NewIgnore()
	ig.Add("*.exe")
	ig.Add("!gguard.exe")

	if !ig.Match("cmd/other.exe", false) {
		t.Error("other.exe should stay ignored")
	}
	if ig.Match("cmd/gguard.exe", false) {
		t.Error("gguard.exe should be re-included by the later pattern")
	}
}

func TestIgnoreGenerousPatterns(t *testing.T) {
	// The set the repository backup uses.
	ig := NewIgnore()
	for _, p := range []string{"dist", ".git", "*.exe"} {
		ig.Add(p)
	}

	ignored := []string{"dist", "dist/gguard.exe", ".git/HEAD", "cmd/gguard/gguard.exe", "gguard.exe"}
	for _, p := range ignored {
		if !ig.Match(p, false) && !ig.Match(p, true) {
			t.Errorf("%q should be ignored", p)
		}
	}

	kept := []string{"cmd/gguard/main.go", "README.md", ".gitignore", "docs/dist.md"}
	for _, p := range kept {
		if ig.Match(p, false) {
			t.Errorf("%q should be kept", p)
		}
	}
}

func TestIgnoreAddFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".ggignore")
	body := "# backup junk\n\ndist/\n*.exe\n\n!keep.exe\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	ig := NewIgnore()
	if err := ig.AddFile(path); err != nil {
		t.Fatalf("AddFile: %v", err)
	}

	if ig.Empty() {
		t.Fatal("the file had patterns, the set should not be empty")
	}
	if !ig.Match("dist", true) {
		t.Error("dist/ should be ignored")
	}
	if ig.Match("keep.exe", false) {
		t.Error("keep.exe should survive the negation")
	}
	if !ig.Match("cmd/x.exe", false) {
		t.Error("*.exe should be ignored")
	}

	// A missing file is fine: the ignore file is optional.
	if err := NewIgnore().AddFile(filepath.Join(dir, "nope")); err != nil {
		t.Errorf("AddFile of a missing file = %v, want nil", err)
	}
}

func TestIgnoreEmptyMatchesNothing(t *testing.T) {
	ig := NewIgnore()
	if !ig.Empty() {
		t.Error("a new set should be empty")
	}
	for _, p := range []string{"dist", "a/b/c.exe", ".git"} {
		if ig.Match(p, true) {
			t.Errorf("%q should not match an empty set", p)
		}
	}
}
