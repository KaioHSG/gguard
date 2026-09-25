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

func TestDebouncerMultipleKeys(t *testing.T) {
	fires := make(chan string, 10)
	d := NewDebouncer(50 * time.Millisecond)
	d.OnFire(func(key string) {
		fires <- key
	})

	d.Trigger("pathA")
	d.Trigger("pathB")

	received := make(map[string]bool)
	for i := 0; i < 2; i++ {
		select {
		case key := <-fires:
			received[key] = true
		case <-time.After(2 * time.Second):
			t.Fatal("timed out")
		}
	}

	if !received["pathA"] || !received["pathB"] {
		t.Errorf("not all keys received: %+v", received)
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