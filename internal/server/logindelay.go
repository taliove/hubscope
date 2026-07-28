package server

import (
	"context"
	"sync"
	"time"
)

// LoginDelayPolicy configures the per-account progressive login delay
// (spec 0011 decision 3): after Threshold failures for the same username
// string within a sliding Window, subsequent failed responses are delayed by
// the successive Backoff entries (the last entry repeats, capping the
// penalty). MaxEntries hard-caps the username map (0 applies the built-in
// default). The zero policy disables the mechanism (nil delayer), mirroring
// the newIPLimiter nil semantics.
type LoginDelayPolicy struct {
	Threshold  int
	Window     time.Duration
	Backoff    []time.Duration
	MaxEntries int
}

// defaultLoginDelayPolicy is the production table: 5 failures within 10
// minutes, then 2s → 4s → 8s (capped) on subsequent failures.
func defaultLoginDelayPolicy() LoginDelayPolicy {
	return LoginDelayPolicy{
		Threshold: 5,
		Window:    10 * time.Minute,
		Backoff:   []time.Duration{2 * time.Second, 4 * time.Second, 8 * time.Second},
	}
}

// loginDelayMaxEntries hard-caps the username map. Without it, a username
// spray could grow the map without bound. Unlike the fail-closed per-IP
// limiter, a full table skips counting for new usernames (the per-IP limiter
// remains the backstop) rather than rejecting legitimate logins.
const loginDelayMaxEntries = 100_000

// loginDelayer counts failed logins per username string in memory (unknown
// usernames count too, so behavior cannot leak account existence) and
// penalizes failures past the threshold. State is process-local and lost on
// restart — an accepted trade-off (spec 0011 decision 3).
type loginDelayer struct {
	mu         sync.Mutex
	threshold  int
	window     time.Duration
	backoff    []time.Duration
	maxEntries int
	keep       int // per-entry timestamp cap: threshold + len(backoff)
	items      map[string]*loginDelayEntry
	sweeps     int
}

// loginDelayEntry tracks recent failure timestamps for one username.
type loginDelayEntry struct {
	failures []time.Time
	seen     time.Time
}

// newLoginDelayer builds the counter; a zero policy disables the mechanism
// (nil). maxEntries<=0 applies the built-in cap.
func newLoginDelayer(p LoginDelayPolicy) *loginDelayer {
	if p.Threshold <= 0 || p.Window <= 0 || len(p.Backoff) == 0 {
		return nil
	}
	if p.MaxEntries <= 0 {
		p.MaxEntries = loginDelayMaxEntries
	}
	return &loginDelayer{
		threshold:  p.Threshold,
		window:     p.Window,
		backoff:    p.Backoff,
		maxEntries: p.MaxEntries,
		keep:       p.Threshold + len(p.Backoff),
		items:      map[string]*loginDelayEntry{},
	}
}

// penalize records one failed login for username and sleeps for the penalty
// the failure earns. The count increment and penalty-band decision happen
// under the mutex (concurrent failures for one username cannot lose counts;
// the band may skew one level high under races — an accepted bound); the
// sleep happens after the lock is released and never holds any DB resource —
// the caller invokes penalize only after all database work is done. The
// sleep is context-aware: a disconnected client releases the goroutine
// early.
func (d *loginDelayer) penalize(ctx context.Context, username string) {
	now := time.Now()

	d.mu.Lock()
	// Evict stale entries opportunistically every 1024 calls so a username
	// spray cannot grow the map without bound.
	d.sweeps++
	if d.sweeps >= 1024 {
		d.sweeps = 0
		cutoff := now.Add(-d.window)
		for name, e := range d.items {
			if e.seen.Before(cutoff) {
				delete(d.items, name)
			}
		}
	}

	e, ok := d.items[username]
	if !ok {
		if len(d.items) >= d.maxEntries {
			// Table full: skip counting rather than failing closed.
			d.mu.Unlock()
			return
		}
		e = &loginDelayEntry{}
		d.items[username] = e
	}
	// Slide the window: drop failures that aged out, then append this one.
	cutoff := now.Add(-d.window)
	kept := e.failures[:0]
	for _, ts := range e.failures {
		if ts.After(cutoff) {
			kept = append(kept, ts)
		}
	}
	e.failures = append(kept, now)
	// Cap the per-entry slice: past threshold+len(backoff) the exact count
	// no longer changes the penalty, and an uncapped slice would let a
	// single sprayed username bypass the map's memory bound.
	if len(e.failures) > d.keep {
		e.failures = e.failures[len(e.failures)-d.keep:]
	}
	e.seen = now
	count := len(e.failures)
	d.mu.Unlock()

	over := count - d.threshold
	if over <= 0 {
		return
	}
	band := over - 1
	if band >= len(d.backoff) {
		band = len(d.backoff) - 1
	}
	delay := d.backoff[band]
	if delay <= 0 {
		return
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}

// reset clears the failure count for username after a successful login.
func (d *loginDelayer) reset(username string) {
	d.mu.Lock()
	delete(d.items, username)
	d.mu.Unlock()
}
