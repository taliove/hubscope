package scheduler

import (
	"sync"
	"time"
)

// Clock abstracts time so the scheduler can be driven by a fake clock in
// tests instead of sleeping in real time.
type Clock interface {
	Now() time.Time
	NewTimer(d time.Duration) Timer
}

// Timer is a single-fire countdown sourced from a Clock.
type Timer interface {
	C() <-chan time.Time
	Stop()
}

// RealClock implements Clock against wall-clock time.
type RealClock struct{}

// Now returns the current wall-clock time.
func (RealClock) Now() time.Time { return time.Now() }

// NewTimer returns a timer backed by time.NewTimer.
func (RealClock) NewTimer(d time.Duration) Timer { return &realTimer{t: time.NewTimer(d)} }

// realTimer adapts *time.Timer to the Timer interface.
type realTimer struct {
	t *time.Timer
}

// C returns the timer's fire channel.
func (r *realTimer) C() <-chan time.Time { return r.t.C }

// Stop stops the timer.
func (r *realTimer) Stop() { r.t.Stop() }

// FakeClock is a manually advanced Clock for tests. Advance moves time
// forward and fires every timer whose deadline has passed. It is safe for
// concurrent use.
type FakeClock struct {
	mu     sync.Mutex
	now    time.Time
	timers map[*fakeTimer]struct{}
}

// NewFakeClock creates a FakeClock starting at the given time.
func NewFakeClock(start time.Time) *FakeClock {
	return &FakeClock{
		now:    start,
		timers: make(map[*fakeTimer]struct{}),
	}
}

// Now returns the fake current time.
func (c *FakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// NewTimer registers a timer that fires once the fake time reaches
// now+d. A non-positive duration fires immediately.
func (c *FakeClock) NewTimer(d time.Duration) Timer {
	c.mu.Lock()
	defer c.mu.Unlock()
	t := &fakeTimer{ch: make(chan time.Time, 1), clock: c}
	if d <= 0 {
		t.ch <- c.now
		return t
	}
	t.deadline = c.now.Add(d)
	c.timers[t] = struct{}{}
	return t
}

// Advance moves the fake time forward by d and fires all timers whose
// deadline has been reached. Sends are non-blocking because each timer
// channel has capacity 1 and fires at most once.
func (c *FakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
	for t := range c.timers {
		if t.deadline.After(c.now) {
			continue
		}
		t.ch <- c.now
		delete(c.timers, t)
	}
}

// TimerCount reports how many timers are currently armed. Tests use it to
// synchronize with the scheduler loop before advancing time.
func (c *FakeClock) TimerCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.timers)
}

// fakeTimer is a Timer registered on a FakeClock.
type fakeTimer struct {
	deadline time.Time
	ch       chan time.Time
	clock    *FakeClock
}

// C returns the channel that receives the fake time when the timer fires.
func (t *fakeTimer) C() <-chan time.Time { return t.ch }

// Stop deregisters the timer from the clock so TimerCount reflects only
// genuinely armed timers and a stopped timer never fires.
func (t *fakeTimer) Stop() {
	t.clock.mu.Lock()
	defer t.clock.mu.Unlock()
	delete(t.clock.timers, t)
}
