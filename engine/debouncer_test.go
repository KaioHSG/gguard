package engine

import (
	"testing"
	"time"
)

func TestDebouncerFires(t *testing.T) {
	fired := make(chan string, 1)
	d := NewDebouncer(50 * time.Millisecond)
	d.OnFire(func(key string) {
		fired <- key
	})

	d.Trigger("test")

	select {
	case key := <-fired:
		if key != "test" {
			t.Errorf("expected 'test', got %q", key)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("debouncer did not fire")
	}
}

func TestDebouncerResetsOnNewEvent(t *testing.T) {
	fires := make(chan string, 10)
	d := NewDebouncer(200 * time.Millisecond)
	d.OnFire(func(key string) {
		fires <- key
	})

	d.Trigger("path1")
	time.Sleep(50 * time.Millisecond)
	d.Trigger("path1")
	time.Sleep(50 * time.Millisecond)
	d.Trigger("path1")

	select {
	case key := <-fires:
		t.Errorf("debouncer fired too early: %q", key)
	case <-time.After(80 * time.Millisecond):
	}

	select {
	case key := <-fires:
		if key != "path1" {
			t.Errorf("expected 'path1', got %q", key)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("debouncer did not fire after reset")
	}
}

// A burst of changes is one fire, carrying the most recent path: running the
// guard once per changed file would mean a build triggers as many runs as the
// files it touched.
func TestDebouncerCoalescesABurst(t *testing.T) {
	fires := make(chan string, 10)
	d := NewDebouncer(100 * time.Millisecond)
	d.OnFire(func(key string) {
		fires <- key
	})

	d.Trigger("pathA")
	d.Trigger("pathB")
	d.Trigger("pathC")

	select {
	case key := <-fires:
		if key != "pathC" {
			t.Errorf("expected the most recent path 'pathC', got %q", key)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("debouncer did not fire")
	}

	select {
	case key := <-fires:
		t.Errorf("a burst must fire once, got a second fire for %q", key)
	case <-time.After(300 * time.Millisecond):
	}
}

func TestDebouncerStop(t *testing.T) {
	fired := false
	d := NewDebouncer(1 * time.Second)
	d.OnFire(func(key string) {
		fired = true
	})

	d.Trigger("path1")
	d.Stop()

	time.Sleep(2 * time.Second)

	if fired {
		t.Error("debouncer fired after stop")
	}
}

func TestDebouncerClearsTimers(t *testing.T) {
	fires := make(chan string, 10)
	d := NewDebouncer(50 * time.Millisecond)
	d.OnFire(func(key string) {
		fires <- key
	})

	d.Trigger("path1")

	select {
	case key := <-fires:
		if key != "path1" {
			t.Errorf("expected 'path1', got %q", key)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timer did not fire")
	}

	d.Trigger("path2")

	select {
	case key := <-fires:
		if key != "path2" {
			t.Errorf("expected 'path2', got %q", key)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timer did not fire after clear")
	}
}
