package engine

import (
	"sync"
	"time"
)

// Debouncer turns a burst of filesystem events into a single fire.
//
// Saving a file, running a build or syncing a repository produces many events
// in a row; the guard should run once, after the tree has been quiet for the
// configured interval, not once per changed file. The most recent path travels
// with the fire, so a script can still say which file triggered it.
type Debouncer struct {
	mu       sync.Mutex
	timer    *time.Timer
	lastKey  string
	interval time.Duration
	onFire   func(string)
}

func NewDebouncer(interval time.Duration) *Debouncer {
	return &Debouncer{interval: interval}
}

func (d *Debouncer) OnFire(fn func(string)) {
	d.onFire = fn
}

// Trigger notes a change and restarts the quiet countdown.
func (d *Debouncer) Trigger(key string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.lastKey = key
	if d.timer != nil {
		d.timer.Stop()
	}

	d.timer = time.AfterFunc(d.interval, func() {
		d.mu.Lock()
		key := d.lastKey
		d.timer = nil
		d.mu.Unlock()

		if d.onFire != nil {
			d.onFire(key)
		}
	})
}

// Stop cancels a pending fire, so shutting down stays quiet.
func (d *Debouncer) Stop() {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.timer != nil {
		d.timer.Stop()
		d.timer = nil
	}
}
