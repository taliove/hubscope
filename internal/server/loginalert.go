package server

import (
	"sort"
	"sync"
	"time"

	"github.com/taliove/hubscope/internal/alerter"
)

// LoginAlertPolicy configures the instance-level brute-force login alert
// (spec 0011 decision 4): when failed logins across the whole site reach
// Threshold within a sliding Window, one Lark alert fires and further
// triggers are suppressed for Cooldown. MaxEntries hard-caps the event
// buffer (0 applies the built-in default). The zero policy disables the
// mechanism (nil tracker), mirroring the loginDelayer nil semantics.
type LoginAlertPolicy struct {
	Threshold  int
	Window     time.Duration
	Cooldown   time.Duration
	MaxEntries int
}

// defaultLoginAlertPolicy is the production policy: 10 failures within 10
// minutes alert once, then stay quiet for 30 minutes (spec 0011 decision 4).
func defaultLoginAlertPolicy() LoginAlertPolicy {
	return LoginAlertPolicy{
		Threshold: 10,
		Window:    10 * time.Minute,
		Cooldown:  30 * time.Minute,
	}
}

// loginAlertMaxEntries hard-caps the sliding-window event buffer so a
// distributed spray cannot grow it without bound. The per-IP login limiter
// bounds the injection rate, so this cap is only a backstop.
const loginAlertMaxEntries = 10_000

// loginAlertUsernameRunes bounds the username kept per event. Usernames come
// from an up-to-1MB request body; unbounded strings would let a spray
// inflate both memory and the alert text. Truncated names merge counts,
// which is acceptable for a top-N overview.
const loginAlertUsernameRunes = 64

// loginFailure is one failed login observed by the tracker.
type loginFailure struct {
	at       time.Time
	username string
	ip       string
}

// loginAlertTracker counts failed logins instance-wide in memory and invokes
// the fire callback when the sliding-window count crosses the threshold, at
// most once per cooldown. State is process-local and lost on restart — an
// accepted trade-off (spec 0011 decision 4), same as the loginDelayer.
type loginAlertTracker struct {
	mu         sync.Mutex
	threshold  int
	window     time.Duration
	cooldown   time.Duration
	maxEntries int
	events     []loginFailure
	lastAlert  time.Time
	fire       func(alerter.LoginFailureSnapshot)
}

// newLoginAlertTracker builds the counter; a zero policy (or a nil callback)
// disables the mechanism (nil). maxEntries<=0 applies the built-in cap.
func newLoginAlertTracker(p LoginAlertPolicy, fire func(alerter.LoginFailureSnapshot)) *loginAlertTracker {
	if p.Threshold <= 0 || p.Window <= 0 || p.Cooldown <= 0 || fire == nil {
		return nil
	}
	if p.MaxEntries <= 0 {
		p.MaxEntries = loginAlertMaxEntries
	}
	return &loginAlertTracker{
		threshold:  p.Threshold,
		window:     p.Window,
		cooldown:   p.Cooldown,
		maxEntries: p.MaxEntries,
		fire:       fire,
	}
}

// record notes one failed login and fires the alert callback when the window
// count crosses the threshold outside the cooldown. It is a pure in-memory
// step (microseconds, no DB resource) so it can run on the login request
// path before the progressive-delay sleep; the callback must never block
// (the Evaluator sends asynchronously).
//
// The cooldown is consumed at the moment the threshold is crossed — whether
// or not a webhook is configured or the send later succeeds — so a sustained
// attack triggers at most one settings lookup and log line per cooldown
// (mirroring the endpoint alerter's "flag flips even when the webhook is
// unconfigured" semantics).
func (t *loginAlertTracker) record(username, ip string) {
	now := time.Now()

	t.mu.Lock()
	// Slide the window: drop failures that aged out, then append this one.
	cutoff := now.Add(-t.window)
	kept := t.events[:0]
	for _, e := range t.events {
		if e.at.After(cutoff) {
			kept = append(kept, e)
		}
	}
	t.events = kept
	if len(t.events) >= t.maxEntries {
		// Buffer full: drop the oldest rather than stop counting — going
		// silent at the peak of an attack is the wrong direction.
		t.events = t.events[1:]
	}
	t.events = append(t.events, loginFailure{
		at:       now,
		username: truncateRunes(username, loginAlertUsernameRunes),
		ip:       ip,
	})

	if len(t.events) < t.threshold || now.Sub(t.lastAlert) < t.cooldown {
		t.mu.Unlock()
		return
	}
	t.lastAlert = now
	snapshot := t.snapshotLocked()
	fire := t.fire
	t.mu.Unlock()

	fire(snapshot)
}

// snapshotLocked freezes the window into the alert payload. Callers hold mu.
func (t *loginAlertTracker) snapshotLocked() alerter.LoginFailureSnapshot {
	return alerter.LoginFailureSnapshot{
		Count:        len(t.events),
		Window:       t.window,
		TopUsernames: topLoginKeys(t.events, func(e loginFailure) string { return e.username }, 3),
		TopIPs:       topLoginKeys(t.events, func(e loginFailure) string { return e.ip }, 3),
	}
}

// topLoginKeys returns the n most frequent non-empty keys across the events,
// count-descending with a lexical tie-break for determinism.
func topLoginKeys(events []loginFailure, key func(loginFailure) string, n int) []string {
	counts := map[string]int{}
	for _, e := range events {
		if k := key(e); k != "" {
			counts[k]++
		}
	}
	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if counts[keys[i]] != counts[keys[j]] {
			return counts[keys[i]] > counts[keys[j]]
		}
		return keys[i] < keys[j]
	})
	if len(keys) > n {
		keys = keys[:n]
	}
	return keys
}

// truncateRunes caps s at n runes.
func truncateRunes(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n])
}
