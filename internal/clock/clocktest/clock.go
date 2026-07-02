// Package clocktest provides a deterministic fake clock for tests.
package clocktest

import (
	"sync"
	"time"

	"github.com/romanornr/delta-works/internal/clock"
)

// Clock is a fake clock.Clock advanced manually with Advance.
type Clock struct {
	mu      sync.Mutex
	now     time.Time
	waiters []*waiter
	tickers []*ticker
}

type waiter struct {
	at time.Time
	ch chan time.Time
}

type ticker struct {
	interval time.Duration
	next     time.Time
	ch       chan time.Time
	stopped  bool
}

// New creates a fake clock starting at the given time.
func New(start time.Time) *Clock { return &Clock{now: start} }

// Now implements clock.Clock.
func (c *Clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// After implements clock.Clock.
func (c *Clock) After(d time.Duration) <-chan time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	w := &waiter{at: c.now.Add(d), ch: make(chan time.Time, 1)}
	c.waiters = append(c.waiters, w)
	return w.ch
}

// NewTicker implements clock.Clock. A non-positive interval would keep
// Advance from ever passing the ticker's next fire time, so it panics the
// same way time.NewTicker does.
func (c *Clock) NewTicker(d time.Duration) clock.Ticker {
	if d <= 0 {
		panic("clocktest: non-positive interval for NewTicker")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	t := &ticker{interval: d, next: c.now.Add(d), ch: make(chan time.Time, 1)}
	c.tickers = append(c.tickers, t)
	return &fakeTicker{c: c, t: t}
}

// Advance moves the clock forward, firing due waiters and tickers.
// Ticker sends are non-blocking (buffered 1), matching time.Ticker's
// drop-on-slow-receiver behavior.
func (c *Clock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	target := c.now.Add(d)
	for {
		// Step to the earliest pending ticker fire, or the target.
		step := target
		for _, t := range c.tickers {
			if !t.stopped && t.next.Before(step) {
				step = t.next
			}
		}
		c.now = step
		for _, w := range c.waiters {
			if !w.at.After(c.now) && w.ch != nil {
				w.ch <- c.now
				w.ch = nil
			}
		}
		for _, t := range c.tickers {
			for !t.stopped && !t.next.After(c.now) {
				select {
				case t.ch <- t.next:
				default:
				}
				t.next = t.next.Add(t.interval)
			}
		}
		if c.now.Equal(target) {
			return
		}
	}
}

type fakeTicker struct {
	c *Clock
	t *ticker
}

func (f *fakeTicker) C() <-chan time.Time { return f.t.ch }

func (f *fakeTicker) Stop() {
	f.c.mu.Lock()
	defer f.c.mu.Unlock()
	f.t.stopped = true
}
