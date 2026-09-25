package engine

import (
	"sync"
	"time"
)

type Debouncer struct {
	mu       sync.Mutex
	timers   map[string]*time.Timer
	interval time.Duration
	onFire   func(string)
}

func NewDebouncer(interval time.Duration) *Debouncer {
	return &Debouncer{
		timers:   make(map[string]*time.Timer),
		interval: interval,
	}
}

func (d *Debouncer) OnFire(fn func(string)) {
	d.onFire = fn
}

func (d *Debouncer) Trigger(key string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if t, ok := d.timers[key]; ok {
		t.Stop()
	}

	d.timers[key] = time.AfterFunc(d.interval, func() {
		d.mu.Lock()
		delete(d.timers, key)
		d.mu.Unlock()

		if d.onFire != nil {
			d.onFire(key)
		}
	})
}

func (d *Debouncer) Stop() {
	d.mu.Lock()
	defer d.mu.Unlock()

	for _, t := range d.timers {
		t.Stop()
	}
	d.timers = make(map[string]*time.Timer)
}