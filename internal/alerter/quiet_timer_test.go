package alerter

import (
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/taliove/hubscope/internal/scheduler"
	"github.com/taliove/hubscope/internal/store"
)

// TestQuietTimerRearmReleasesGoroutines pins the supersede-path goroutine
// release (spec 0017 campaign review): every re-arm of the quiet-boundary
// timer supersedes the previous timer, and the superseded waiter goroutine
// must exit via quietDone instead of parking forever on the stopped timer's
// channel. The assertion is a goroutine-count settle, and it is
// deterministic: a leaked goroutine never exits, so with the leak the count
// can never return to baseline; released goroutines observe an already
// closed done channel and exit as soon as they schedule, so the bounded
// wait only absorbs scheduler delay.
//
// The test is in-package by necessity — a parked goroutine is not
// observable at the W1 HTTP seam — and drives only the public-in-package
// re-arm entry point (syncQuietTimerLocked) through real settings writes,
// no mocks.
func TestQuietTimerRearmReleasesGoroutines(t *testing.T) {
	db, err := store.Open(t.TempDir() + "/quiet-timer.db")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	e := NewEvaluator(db, NewLarkSender())
	// 12:00 local, outside the 23:00–07:00 window: the next boundary is hours
	// ahead, so every re-arm below hits the supersede path (never the
	// in-flight fire guard).
	clock := scheduler.NewFakeClock(time.Date(2026, time.July, 29, 12, 0, 0, 0, time.Local))
	e.UseClock(clock)

	if err := db.SetSettingBool(store.SettingQuietHoursEnabled, true); err != nil {
		t.Fatalf("enable quiet hours: %v", err)
	}

	sync := func() {
		e.mu.Lock()
		defer e.mu.Unlock()
		e.syncQuietTimerLocked()
	}

	sync() // initial arm: boundary 23:00, one waiter goroutine
	runtime.GC()
	baseline := runtime.NumGoroutine()

	// Ten re-arms, each moving the armed boundary (start 22 ↔ 23 flips the
	// next boundary between 22:00 and 23:00), each superseding the previous
	// timer.
	const rearms = 10
	for i := 0; i < rearms; i++ {
		if err := db.SetSettingInt(store.SettingQuietHoursStart, 22+i%2); err != nil {
			t.Fatalf("write quiet start: %v", err)
		}
		sync()
	}

	// Disarm the final timer so a passing run ends with zero quiet-timer
	// goroutines (one below the baseline that included the initial waiter).
	if err := db.SetSettingBool(store.SettingQuietHoursEnabled, false); err != nil {
		t.Fatalf("disable quiet hours: %v", err)
	}
	sync()

	deadline := time.Now().Add(5 * time.Second)
	for runtime.NumGoroutine() >= baseline {
		if time.Now().After(deadline) {
			t.Fatalf("superseded quiet-timer goroutines did not exit: baseline %d, now %d",
				baseline, runtime.NumGoroutine())
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestQuietTimerInflightFireNotCancelled guards the other half of the
// invariant: when the armed boundary has been reached (fire buffered, not
// yet consumed), a re-arm must NOT cancel the crossing — the boundary
// handler still runs and (window just ended) delivers the summary. The
// handler's execution is observed through a real delivery to a stub Lark
// endpoint: a deferred score_drop rides the window-end summary, so the
// summary is non-empty and its POST is the observable trace. (A weaker
// version of this test observed only the re-armed boundary — and passed
// with the guard mutated out, because the mutating sync's own supersede
// re-arm produces the identical terminal state; check GH HIGH-1.)
//
// e.mu is held across Advance + sync so the waiter goroutine cannot consume
// the fire in between: with the guard, the waiter later finds only
// timer.C() ready and runs the handler (deterministic green); with the
// guard mutated out, sync disarms and re-arms under the same lock, and the
// waiter — whichever select branch it takes — either exits via done or
// fails the supersede check, so the handler never runs and the summary
// never arrives (deterministic red).
func TestQuietTimerInflightFireNotCancelled(t *testing.T) {
	db, err := store.Open(t.TempDir() + "/quiet-timer-inflight.db")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	// Minimal stub Lark endpoint: 200 on every post, bodies recorded.
	var mu sync.Mutex
	var bodies []string
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		bodies = append(bodies, string(body))
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer stub.Close()

	e := NewEvaluator(db, NewLarkSender())
	// 06:59 local, inside the 23:00–07:00 window: the armed boundary is
	// 07:00, one minute out.
	clock := scheduler.NewFakeClock(time.Date(2026, time.July, 30, 6, 59, 0, 0, time.Local))
	e.UseClock(clock)

	if err := db.SetSettingBool(store.SettingQuietHoursEnabled, true); err != nil {
		t.Fatalf("enable quiet hours: %v", err)
	}
	if err := db.SetSetting(store.SettingLarkWebhookURL, stub.URL); err != nil {
		t.Fatalf("set webhook: %v", err)
	}

	// One deferred score_drop, as deferScoreDropLocked would leave it: the
	// event persisted with sent_ok=false, the frozen text queued in memory —
	// so the window-end summary has content and must be delivered.
	const dropText = "【HubScope】评估分数大跌:模型 inflight-model 本轮评估(Campaign #2)对比上一轮,1 个评估集得分大跌(阈值 0.2):\n·「GSM8K」1.00 → 0.00(跌 1.00)"
	event, err := db.CreateAlertEvent(store.AlertEvent{
		Kind:    store.AlertKindScoreDrop,
		Message: dropText,
		SentOK:  false,
	})
	if err != nil {
		t.Fatalf("record deferred score_drop: %v", err)
	}
	e.mu.Lock()
	e.quietScoreDrops = []quietScoreDrop{{eventID: event.ID, text: dropText}}
	e.syncQuietTimerLocked() // arm for 07:00
	clock.Advance(time.Minute)
	// 07:00 — the fire is buffered on the timer channel while we still hold
	// e.mu, so the waiter cannot have consumed it. This sync hits the
	// in-flight guard: it must leave the armed timer untouched.
	e.syncQuietTimerLocked()
	e.mu.Unlock()

	// The buffered fire must still be consumed: the handler runs, observes
	// the window ended, and delivers the summary carrying the deferred drop.
	deadline := time.Now().Add(5 * time.Second)
	for {
		mu.Lock()
		received := len(bodies) > 0 && strings.Contains(bodies[0], "inflight-model")
		mu.Unlock()
		if received {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("in-flight quiet-boundary fire was swallowed: stub received %d messages, want the quiet summary naming inflight-model", len(bodies))
		}
		time.Sleep(10 * time.Millisecond)
	}
}
