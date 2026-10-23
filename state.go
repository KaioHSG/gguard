package main

import "sync"

// GuardState describes what a guard is doing right now. It drives the tray
// icon color and the status output.
type GuardState int

const (
	// StateIdle: the guard has not run yet (or is stopped). Shown as yellow.
	StateIdle GuardState = iota
	// StateRunning: the guard is executing right now. Shown as blue.
	StateRunning
	// StateOK: the last run finished successfully. Shown as green.
	StateOK
	// StateError: the last run failed. Shown as red.
	StateError
)

func (s GuardState) String() string {
	switch s {
	case StateRunning:
		return "running"
	case StateOK:
		return "ok"
	case StateError:
		return "error"
	default:
		return "idle"
	}
}

type guardRuntime struct {
	Watch   string
	State   GuardState
	Runs    int
	LastErr string
}

// StatusTracker keeps the live state of every guard so the tray and the
// status file can be updated as runs happen.
type StatusTracker struct {
	mu        sync.Mutex
	order     []string
	guards    map[string]*guardRuntime
	listeners []func()
}

func NewStatusTracker() *StatusTracker {
	return &StatusTracker{guards: make(map[string]*guardRuntime)}
}

// AddListener registers a callback fired (outside the lock) whenever any guard
// changes state. Used by the tray and the status file writer.
func (t *StatusTracker) AddListener(fn func()) {
	t.mu.Lock()
	t.listeners = append(t.listeners, fn)
	t.mu.Unlock()
}

func (t *StatusTracker) ensure(name, watch string) *guardRuntime {
	g, ok := t.guards[name]
	if !ok {
		g = &guardRuntime{State: StateIdle}
		t.guards[name] = g
		t.order = append(t.order, name)
	}
	if watch != "" {
		g.Watch = watch
	}
	return g
}

// Add registers a guard as idle (not yet run).
func (t *StatusTracker) Add(name, watch string) {
	t.mu.Lock()
	t.ensure(name, watch)
	t.mu.Unlock()
	t.notify()
}

// SetRunning marks a guard as currently executing.
func (t *StatusTracker) SetRunning(name string) {
	t.mu.Lock()
	t.ensure(name, "").State = StateRunning
	t.mu.Unlock()
	t.notify()
}

// SetResult records the outcome of a run.
func (t *StatusTracker) SetResult(name string, err error) {
	t.mu.Lock()
	g := t.ensure(name, "")
	g.Runs++
	if err != nil {
		g.State = StateError
		g.LastErr = err.Error()
	} else {
		g.State = StateOK
		g.LastErr = ""
	}
	t.mu.Unlock()
	t.notify()
}

// Aggregate returns the combined state across every guard, using priority
// running > error > ok > idle.
func (t *StatusTracker) Aggregate() GuardState {
	t.mu.Lock()
	defer t.mu.Unlock()

	var running, failed, ok bool
	for _, g := range t.guards {
		switch g.State {
		case StateRunning:
			running = true
		case StateError:
			failed = true
		case StateOK:
			ok = true
		}
	}
	switch {
	case running:
		return StateRunning
	case failed:
		return StateError
	case ok:
		return StateOK
	default:
		return StateIdle
	}
}

// Entries returns a snapshot of every guard in registration order.
func (t *StatusTracker) Entries() []statusEntry {
	t.mu.Lock()
	defer t.mu.Unlock()

	entries := make([]statusEntry, 0, len(t.order))
	for _, name := range t.order {
		g := t.guards[name]
		entries = append(entries, statusEntry{
			Name:      name,
			Watch:     g.Watch,
			State:     g.State.String(),
			Runs:      g.Runs,
			LastError: g.LastErr,
		})
	}
	return entries
}

// notify fires every listener without holding the lock, so listeners are free
// to call back into the tracker.
func (t *StatusTracker) notify() {
	t.mu.Lock()
	listeners := make([]func(), len(t.listeners))
	copy(listeners, t.listeners)
	t.mu.Unlock()

	for _, fn := range listeners {
		fn()
	}
}
