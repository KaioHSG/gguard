package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// The reload trigger is a fingerprint of the scripts and their config: any
// change to what a reload reads has to show up in it.
func TestConfigSignatureNoticesChanges(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "guards.json")
	script := filepath.Join(dir, "demo.ggs")

	if err := os.WriteFile(cfg, []byte(`{"demo":{"autostart":true}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(script, []byte("guard \"Demo\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	src := routineSources{scriptDirs: []string{dir}, globalCfg: cfg}
	before := configSignature(src)

	// Rewriting a script counts, even when the size stays the same: the
	// modification time is part of the signature.
	time.Sleep(10 * time.Millisecond)
	if err := os.WriteFile(script, []byte("guard \"Demo2\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	edited := configSignature(src)
	if edited == before {
		t.Error("editing a script did not change the signature")
	}

	// So does a new routine, a deleted one, and a config change.
	if err := os.WriteFile(filepath.Join(dir, "other.ggs"), []byte("guard \"Other\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	added := configSignature(src)
	if added == edited {
		t.Error("adding a script did not change the signature")
	}

	if err := os.Remove(filepath.Join(dir, "other.ggs")); err != nil {
		t.Fatal(err)
	}
	removed := configSignature(src)
	if removed == added {
		t.Error("removing a script did not change the signature")
	}
	if removed != edited {
		t.Error("removing the added script should be back to the edited state")
	}

	time.Sleep(10 * time.Millisecond)
	if err := os.WriteFile(cfg, []byte(`{"demo":{"autostart":false}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if configSignature(src) == removed {
		t.Error("editing guards.json did not change the signature")
	}

	// A missing directory is a state of its own, not an error.
	missing := configSignature(routineSources{scriptDirs: []string{filepath.Join(dir, "nope")}, globalCfg: cfg})
	if missing == configSignature(src) {
		t.Error("a missing script directory should not look like the current state")
	}
}

func TestTrackerPruneKeepsKnownGuards(t *testing.T) {
	tracker := NewStatusTracker()
	tracker.Add("keep", "/tmp/keep")
	tracker.Add("gone", "/tmp/gone")
	tracker.SetResult("keep", nil)
	tracker.SetResult("gone", nil)

	tracker.Prune([]string{"keep"})

	if names := tracker.Names(); len(names) != 1 || names[0] != "keep" {
		t.Fatalf("names = %v, want [keep]", names)
	}

	// The survivor keeps its state and run count.
	entries := tracker.Entries()
	if len(entries) != 1 || entries[0].Runs != 1 || entries[0].State != "ok" {
		t.Errorf("entries = %+v, want the kept guard with its run recorded", entries)
	}
}
