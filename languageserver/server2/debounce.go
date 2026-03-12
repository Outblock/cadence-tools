package server2

import (
	"sync"
	"time"
)

// Debouncer coalesces rapid calls for the same key, executing only the
// last callback after the delay expires. Each new Trigger resets the timer.
type Debouncer struct {
	delay  time.Duration
	mu     sync.Mutex
	timers map[string]*time.Timer
}

// NewDebouncer creates a Debouncer that waits for delay after the last
// Trigger before invoking the callback.
func NewDebouncer(delay time.Duration) *Debouncer {
	return &Debouncer{
		delay:  delay,
		timers: make(map[string]*time.Timer),
	}
}

// Trigger schedules fn to run after the debounce delay for key.
// If a previous trigger for the same key is still pending, it is cancelled
// and replaced by fn.
func (d *Debouncer) Trigger(key string, fn func()) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if t, ok := d.timers[key]; ok {
		t.Stop()
	}

	d.timers[key] = time.AfterFunc(d.delay, func() {
		d.mu.Lock()
		delete(d.timers, key)
		d.mu.Unlock()
		fn()
	})
}

// Stop cancels all pending timers.
func (d *Debouncer) Stop() {
	d.mu.Lock()
	defer d.mu.Unlock()

	for key, t := range d.timers {
		t.Stop()
		delete(d.timers, key)
	}
}
