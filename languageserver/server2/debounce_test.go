package server2

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestDebouncer_CoalescesRapidCalls(t *testing.T) {
	d := NewDebouncer(50 * time.Millisecond)
	defer d.Stop()

	var count atomic.Int32

	for i := 0; i < 10; i++ {
		d.Trigger("key", func() {
			count.Add(1)
		})
		time.Sleep(10 * time.Millisecond)
	}

	// Wait for the debounce to fire after the last trigger
	time.Sleep(100 * time.Millisecond)

	if got := count.Load(); got != 1 {
		t.Errorf("expected callback to fire once, got %d", got)
	}
}

func TestDebouncer_IndependentKeys(t *testing.T) {
	d := NewDebouncer(50 * time.Millisecond)
	defer d.Stop()

	var countA, countB atomic.Int32

	d.Trigger("a", func() { countA.Add(1) })
	d.Trigger("b", func() { countB.Add(1) })

	time.Sleep(100 * time.Millisecond)

	if got := countA.Load(); got != 1 {
		t.Errorf("expected key A callback once, got %d", got)
	}
	if got := countB.Load(); got != 1 {
		t.Errorf("expected key B callback once, got %d", got)
	}
}

func TestDebouncer_CancelsPrevious(t *testing.T) {
	d := NewDebouncer(50 * time.Millisecond)
	defer d.Stop()

	var mu sync.Mutex
	var result string

	d.Trigger("key", func() {
		mu.Lock()
		defer mu.Unlock()
		result = "first"
	})

	// Trigger again before the first fires, replacing the callback
	time.Sleep(10 * time.Millisecond)
	d.Trigger("key", func() {
		mu.Lock()
		defer mu.Unlock()
		result = "second"
	})

	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if result != "second" {
		t.Errorf("expected result %q, got %q", "second", result)
	}
}

func TestDebouncer_StopCancelsPending(t *testing.T) {
	d := NewDebouncer(50 * time.Millisecond)

	var count atomic.Int32

	d.Trigger("key", func() { count.Add(1) })
	d.Stop()

	time.Sleep(100 * time.Millisecond)

	if got := count.Load(); got != 0 {
		t.Errorf("expected no callbacks after Stop, got %d", got)
	}
}
