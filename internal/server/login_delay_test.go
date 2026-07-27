package server_test

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/taliove/hubscope/internal/server"
	"github.com/taliove/hubscope/internal/store"
)

// These tests cover spec 0010 decision 3 (per-account progressive login
// delay, ticket 81) at the W1 seam: they only observe HTTP behavior (status
// codes, error messages, response-time magnitude) and never assert internal
// counter state. The delay table is injected via WithLoginDelay at
// millisecond scale (the WithRateLimits precedent), and per-IP rate limits
// are disabled so repeated attempts from one test IP are not answered 429
// before the delay mechanism can act.

// delayTestServer starts the API server with per-IP rate limits disabled and
// the given millisecond-scale login-delay policy.
func delayTestServer(t *testing.T, db *store.DB, policy server.LoginDelayPolicy) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(server.New(db,
		server.WithRateLimits(server.RateLimits{}),
		server.WithLoginDelay(policy),
	))
	t.Cleanup(ts.Close)
	return ts
}

// loginTimed posts one login attempt and reports the status, the error
// message (empty on success), and the wall-clock duration.
func loginTimed(t *testing.T, baseURL, username, password string) (int, string, time.Duration) {
	t.Helper()
	start := time.Now()
	resp := postRaw(t, http.DefaultClient, baseURL+"/api/auth/login",
		`{"username":`+strconv.Quote(username)+`,"password":`+strconv.Quote(password)+`}`)
	elapsed := time.Since(start)
	status := resp.StatusCode
	msg := ""
	if status == http.StatusOK {
		resp.Body.Close()
	} else {
		msg = readErrorMessage(t, resp)
	}
	return status, msg, elapsed
}

// TestLoginDelayThresholdAndBackoff verifies failures within the threshold
// answer immediately while failures past it earn escalating penalties
// (spec test decision 1).
func TestLoginDelayThresholdAndBackoff(t *testing.T) {
	db := openTempDB(t)
	ts := delayTestServer(t, db, server.LoginDelayPolicy{
		Threshold: 3,
		Window:    time.Minute,
		Backoff:   []time.Duration{60 * time.Millisecond, 120 * time.Millisecond, 240 * time.Millisecond},
	})

	// Within the threshold, failures answer immediately.
	for i := 1; i <= 3; i++ {
		status, _, elapsed := loginTimed(t, ts.URL, "admin", "wrong-password")
		if status != http.StatusUnauthorized {
			t.Fatalf("attempt %d: expected 401, got %d", i, status)
		}
		if elapsed >= 50*time.Millisecond {
			t.Fatalf("attempt %d within threshold: expected no delay, took %v", i, elapsed)
		}
	}
	// Failures past the threshold earn escalating penalties (one-sided lower
	// bounds; no strict upper bound, to stay robust against CI jitter).
	if _, _, elapsed := loginTimed(t, ts.URL, "admin", "wrong-password"); elapsed < 55*time.Millisecond {
		t.Fatalf("attempt 4 past threshold: expected ~60ms penalty, took %v", elapsed)
	}
	if _, _, elapsed := loginTimed(t, ts.URL, "admin", "wrong-password"); elapsed < 110*time.Millisecond {
		t.Fatalf("attempt 5 past threshold: expected ~120ms penalty, took %v", elapsed)
	}
}

// TestLoginDelayCorrectPasswordImmediateAndResets verifies the password is
// verified before any penalty is applied — a correct password always passes
// immediately regardless of the counter state — and that a successful login
// resets the counter (spec test decision 2).
func TestLoginDelayCorrectPasswordImmediateAndResets(t *testing.T) {
	db := openTempDB(t)
	ts := delayTestServer(t, db, server.LoginDelayPolicy{
		Threshold: 3,
		Window:    time.Minute,
		Backoff:   []time.Duration{60 * time.Millisecond, 120 * time.Millisecond},
	})

	// Four failures: the fourth is past the threshold and penalized.
	for i := 1; i <= 4; i++ {
		loginTimed(t, ts.URL, "admin", "wrong-password")
	}
	// The correct password is never delayed, even with a hot counter.
	status, _, elapsed := loginTimed(t, ts.URL, "admin", testAdminPassword)
	if status != http.StatusOK {
		t.Fatalf("correct password: expected 200, got %d", status)
	}
	if elapsed >= 50*time.Millisecond {
		t.Fatalf("correct password must never be delayed, took %v", elapsed)
	}
	// Success reset the counter: the next failure is back to the no-delay band.
	if _, _, elapsed := loginTimed(t, ts.URL, "admin", "wrong-password"); elapsed >= 50*time.Millisecond {
		t.Fatalf("failure after successful login: expected counter reset, took %v", elapsed)
	}
}

// TestLoginDelayNoEnumerationBypass verifies a nonexistent username and a
// real username fail identically — same status, same message, same delay
// pattern (spec test decision 3).
func TestLoginDelayNoEnumerationBypass(t *testing.T) {
	db := openTempDB(t)
	ts := delayTestServer(t, db, server.LoginDelayPolicy{
		Threshold: 2,
		Window:    time.Minute,
		Backoff:   []time.Duration{80 * time.Millisecond},
	})

	realElapsed := make([]time.Duration, 0, 3)
	ghostElapsed := make([]time.Duration, 0, 3)
	for i := 1; i <= 3; i++ {
		rStatus, rMsg, rEl := loginTimed(t, ts.URL, "admin", "wrong-password")
		gStatus, gMsg, gEl := loginTimed(t, ts.URL, "ghost", "wrong-password")
		if rStatus != gStatus || rMsg != gMsg {
			t.Fatalf("attempt %d: real (%d, %q) and ghost (%d, %q) differ", i, rStatus, rMsg, gStatus, gMsg)
		}
		realElapsed = append(realElapsed, rEl)
		ghostElapsed = append(ghostElapsed, gEl)
	}
	// Same delay pattern: both fast within the threshold, both penalized past it.
	for i := 0; i < 2; i++ {
		if realElapsed[i] >= 50*time.Millisecond || ghostElapsed[i] >= 50*time.Millisecond {
			t.Fatalf("attempt %d within threshold must be fast for both: real %v, ghost %v",
				i+1, realElapsed[i], ghostElapsed[i])
		}
	}
	if realElapsed[2] < 70*time.Millisecond || ghostElapsed[2] < 70*time.Millisecond {
		t.Fatalf("attempt 3 past threshold must be penalized for both: real %v, ghost %v",
			realElapsed[2], ghostElapsed[2])
	}
}

// TestLoginDelayWindowDecays verifies counts decay naturally once failures
// slide out of the window (acceptance: no external reset needed).
func TestLoginDelayWindowDecays(t *testing.T) {
	db := openTempDB(t)
	ts := delayTestServer(t, db, server.LoginDelayPolicy{
		Threshold: 2,
		Window:    150 * time.Millisecond,
		Backoff:   []time.Duration{80 * time.Millisecond},
	})

	loginTimed(t, ts.URL, "admin", "wrong-password")
	loginTimed(t, ts.URL, "admin", "wrong-password")
	if _, _, elapsed := loginTimed(t, ts.URL, "admin", "wrong-password"); elapsed < 70*time.Millisecond {
		t.Fatalf("attempt 3 past threshold: expected ~80ms penalty, took %v", elapsed)
	}
	// Let the window slide; the count decays and failures answer fast again.
	time.Sleep(250 * time.Millisecond)
	if _, _, elapsed := loginTimed(t, ts.URL, "admin", "wrong-password"); elapsed >= 50*time.Millisecond {
		t.Fatalf("failure after the window slid: expected no delay, took %v", elapsed)
	}
}

// TestLoginDelayEntryCapSkipsNewUsernames verifies the username map is
// hard-capped and, unlike the fail-closed per-IP limiter, a full table skips
// counting for never-seen usernames — they are still answered 401, never
// delayed, and nothing panics (the per-IP limiter remains the backstop).
func TestLoginDelayEntryCapSkipsNewUsernames(t *testing.T) {
	db := openTempDB(t)
	ts := delayTestServer(t, db, server.LoginDelayPolicy{
		Threshold:  1,
		Window:     time.Minute,
		Backoff:    []time.Duration{80 * time.Millisecond},
		MaxEntries: 2,
	})

	// Two usernames fill the table; both are penalized past the threshold.
	for _, name := range []string{"first", "second"} {
		loginTimed(t, ts.URL, name, "wrong-password")
		status, _, elapsed := loginTimed(t, ts.URL, name, "wrong-password")
		if status != http.StatusUnauthorized {
			t.Fatalf("%s: expected 401, got %d", name, status)
		}
		if elapsed < 70*time.Millisecond {
			t.Fatalf("%s past threshold: expected ~80ms penalty, took %v", name, elapsed)
		}
	}
	// The table is full: a never-seen username is skipped (no counting, no
	// delay) but still rejected — no fail-closed, no panic.
	for i := 1; i <= 3; i++ {
		status, _, elapsed := loginTimed(t, ts.URL, "third", "wrong-password")
		if status != http.StatusUnauthorized {
			t.Fatalf("third attempt %d: expected 401, got %d", i, status)
		}
		if elapsed >= 50*time.Millisecond {
			t.Fatalf("third (over cap) attempt %d must not be delayed, took %v", i, elapsed)
		}
	}
}
