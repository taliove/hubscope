package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/taliove/hubscope/internal/scheduler"
	"github.com/taliove/hubscope/internal/server"
	"github.com/taliove/hubscope/internal/store"
)

// quiet_hours_test.go covers spec 0017 ticket 4 (GH #67): the configurable
// daily quiet-hours window. Inside the window the window-flush and the
// score_drop send are gated — the state machine and the event log are not
// (transitions still record events with sent_ok=false, "delivery
// unconfirmed") — and when the window ends a single quiet_summary card
// reports what still needs action: still-open endpoints whose alert was
// never delivered (their anchor event's sent_ok is false), still-open vendor
// groups on the same rule, and the score_drop events deferred during the
// window. An empty summary is never sent; login brute-force alerts are
// exempt and stay immediate.
//
// All tests are black-box at the W1 seam: manual probe rounds / eval
// campaigns / login attempts drive behavior, the injected FakeClock (pinned
// to explicit local-time instants, so cross-midnight assertions never depend
// on the runner's TZ) crosses the window boundaries, and assertions read
// only the Lark stub and GET /api/alerts + GET/PUT /api/settings.
//
// Test timeline convention: quiet hours 23:00–07:00, clock starts at
// 2026-07-29 23:30 local (inside the window) unless a test says otherwise.

// quietTestStart / quietTestEnd are the quiet window used across this file.
const (
	quietTestStart = 23
	quietTestEnd   = 7
)

// quietClockStart is 2026-07-29 23:30 in the server-local zone — inside the
// 23:00–07:00 window under every TZ the test suite may run in, because the
// boundary math and this instant share the same location.
func quietClockStart() time.Time {
	return time.Date(2026, time.July, 29, 23, 30, 0, 0, time.Local)
}

// configureQuietHours writes the three quiet-hours settings keys.
func configureQuietHours(t *testing.T, ts *httptest.Server, enabled bool, start, end int) {
	t.Helper()
	resp := doPut(t, ts.URL+"/api/settings", map[string]interface{}{
		"quiet_hours_enabled": enabled,
		"quiet_hours_start":   start,
		"quiet_hours_end":     end,
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("put quiet-hours settings: expected 200, got %d", resp.StatusCode)
	}
}

// quietSummaryEvents returns only the quiet_summary alert events.
func quietSummaryEvents(t *testing.T, ts *httptest.Server) []map[string]interface{} {
	t.Helper()
	return alertEventsOfKind(t, ts, "quiet_summary")
}

// TestQuietHoursSuppressesFlushThenSummarizes is the AC1 spine: inside the
// window a down transition records its event but the flush sends nothing;
// crossing exactly onto the 07:00 boundary delivers exactly one
// quiet_summary naming the still-faulty endpoint, writes the anchor event's
// sent_ok back, and records the summary event itself.
func TestQuietHoursSuppressesFlushThenSummarizes(t *testing.T) {
	db := openTempDB(t)
	clock := scheduler.NewFakeClock(quietClockStart())
	ts := newAlertClockServer(t, db, clock)
	lark := newStubLarkServer(t)
	stubHub := newStubHubServer()
	defer stubHub.Close()

	configureWebhook(t, ts, lark, true)
	configureQuietHours(t, ts, true, quietTestStart, quietTestEnd)

	endpointID := createProbedEndpoint(t, ts, "Quiet Hub", stubHub.URL, "quiet-model")
	stubHub.SetMode("error_503")

	// The down transition is decided at 23:30, inside the window: the event
	// lands at once (sent_ok=false, "delivery unconfirmed"), nothing is sent.
	runProbeRound(t, ts, endpointID)
	runProbeRound(t, ts, endpointID)
	events := listAlerts(t, ts, "")
	if len(events) != 1 || events[0]["kind"].(string) != "down" {
		t.Fatalf("quiet-hours transition should still record the down event, got %v", events)
	}
	if events[0]["sent_ok"].(bool) != false {
		t.Error("event recorded inside quiet hours: expected sent_ok false (delivery unconfirmed)")
	}
	if got := len(lark.messages()); got != 0 {
		t.Fatalf("inside quiet hours: expected no messages, got %d", got)
	}

	// The 60s window flushes inside the window: still nothing. The flush's
	// quiet gate evaluates the window's scheduled fire time, so nothing can
	// arrive even before the flush goroutine runs — no settle needed.
	clock.Advance(alertWindowForTest) // 23:31
	if got := len(lark.messages()); got != 0 {
		t.Fatalf("window flush inside quiet hours must not send, got %d messages", got)
	}

	// Advance exactly onto the 07:00 boundary (07:00 is no longer quiet):
	// exactly one quiet_summary arrives, naming the still-faulty endpoint.
	clock.Advance(7*time.Hour + 29*time.Minute) // 23:31 + 7h29m = 07:00:00
	msgs := waitForLarkMessages(t, lark, 1)
	for _, want := range []string{"quiet-model", "Quiet Hub", "anthropic", "静默"} {
		if !strings.Contains(msgs[0], want) {
			t.Errorf("quiet summary should contain %q, got: %s", want, msgs[0])
		}
	}

	waitForAlertEvents(t, ts, 2) // down + quiet_summary: summary settled
	if got := len(lark.messages()); got != 1 {
		t.Fatalf("quiet end: expected exactly 1 message (the summary), got %d", got)
	}
	cards := lark.cards()
	if len(cards) != 1 {
		t.Fatalf("expected 1 card, got %d", len(cards))
	}
	summary := cards[0]
	if summary.Template != "red" {
		t.Errorf("quiet summary card template: expected red (still-faulty is bad news), got %q", summary.Template)
	}
	if summary.Title != "静默摘要 · HubScope" {
		t.Errorf("quiet summary card title: got %q", summary.Title)
	}
	for _, want := range []string{"Quiet Hub", "quiet-model", "anthropic"} {
		if !strings.Contains(summary.Detail, want) {
			t.Errorf("quiet summary detail should contain %q, got: %s", want, summary.Detail)
		}
	}

	// The still-open endpoint's anchor event is confirmed by the summary;
	// the summary itself lands as one hub-less quiet_summary event carrying
	// the actual plain text sent.
	downEvents := alertEventsOfKind(t, ts, "down")
	if len(downEvents) != 1 || downEvents[0]["sent_ok"].(bool) != true {
		t.Errorf("down anchor event should be written back sent_ok=true by the summary, got %v", downEvents)
	}
	qs := quietSummaryEvents(t, ts)
	if len(qs) != 1 {
		t.Fatalf("expected 1 quiet_summary event, got %d", len(qs))
	}
	if qs[0]["endpoint_id"] != nil {
		t.Errorf("quiet_summary event should be hub-less, got endpoint_id %v", qs[0]["endpoint_id"])
	}
	if qs[0]["sent_ok"].(bool) != true {
		t.Error("quiet_summary event should record the real delivery result (true)")
	}
	qsMessage := qs[0]["message"].(string)
	if !strings.HasPrefix(qsMessage, "【HubScope】") || strings.Contains(qsMessage, "msg_type") {
		t.Errorf("quiet_summary event message must stay plain text, got: %v", qsMessage)
	}
	if !strings.Contains(qsMessage, "quiet-model") {
		t.Errorf("quiet_summary event message should name the model, got: %s", qsMessage)
	}

	// No repeat across further periods: the endpoint is still open but its
	// alert is now delivered (anchor sent_ok=true), so crossing the next
	// quiet night sends nothing more. A second endpoint driven down outside
	// quiet proves the pipeline is otherwise live (its card arrives) — and
	// it, too, is not re-summarized the next morning.
	clock.Advance(2 * time.Hour) // 09:00, outside the window
	stubDay := newStubHubServer()
	defer stubDay.Close()
	ep2 := createProbedEndpoint(t, ts, "Day Hub", stubDay.URL, "day-model")
	stubDay.SetMode("error_503")
	runProbeRound(t, ts, ep2) // still failing hub: failures 1-2
	runProbeRound(t, ts, ep2) // down transition at 09:00, outside quiet
	clock.Advance(alertWindowForTest)
	msgs = waitForLarkMessages(t, lark, 2)
	if !strings.Contains(msgs[1], "day-model") {
		t.Errorf("outside quiet hours the aggregated card should still send, got: %s", msgs[1])
	}

	// Through the next quiet night: 09:01 → 23:31 (inside) → 07:31 next day.
	// Both endpoints stay open but both alerts were delivered — the summary
	// would be empty, so nothing is sent.
	clock.Advance(14*time.Hour + 30*time.Minute) // 23:31
	clock.Advance(8 * time.Hour)                 // 07:31 next day
	// The boundary handler is asynchronous and an empty summary leaves no
	// observable trace by design; give it a bounded moment to (not) fire.
	time.Sleep(100 * time.Millisecond)
	if got := len(lark.messages()); got != 2 {
		t.Fatalf("already-delivered alerts must not be re-summarized the next quiet end, got %d messages", got)
	}
	if got := quietSummaryEvents(t, ts); len(got) != 1 {
		t.Fatalf("expected still exactly 1 quiet_summary event, got %d", len(got))
	}
}

// TestQuietHoursSummarySkipsSelfHealed: an endpoint that goes down and
// recovers entirely inside the window never reaches the summary — only
// endpoints still faulty at the window end are listed.
func TestQuietHoursSummarySkipsSelfHealed(t *testing.T) {
	db := openTempDB(t)
	clock := scheduler.NewFakeClock(quietClockStart())
	ts := newAlertClockServer(t, db, clock)
	lark := newStubLarkServer(t)
	stubStay := newStubHubServer()
	defer stubStay.Close()
	stubHeal := newStubHubServer()
	defer stubHeal.Close()

	configureWebhook(t, ts, lark, true)
	configureQuietHours(t, ts, true, quietTestStart, quietTestEnd)

	epStay := createProbedEndpoint(t, ts, "Stay Hub", stubStay.URL, "stay-model")
	epHeal := createProbedEndpoint(t, ts, "Heal Hub", stubHeal.URL, "heal-model")
	stubStay.SetMode("error_503")
	stubHeal.SetMode("error_503")

	// Both endpoints go down inside the window; the flush holds.
	runProbeRound(t, ts, epStay)
	runProbeRound(t, ts, epStay)
	runProbeRound(t, ts, epHeal)
	runProbeRound(t, ts, epHeal)
	clock.Advance(alertWindowForTest)
	if got := len(lark.messages()); got != 0 {
		t.Fatalf("inside quiet hours: expected no messages, got %d", got)
	}

	// epHeal recovers inside the same quiet night; that flush holds too.
	stubHeal.SetMode("success")
	runProbeRound(t, ts, epHeal)
	clock.Advance(alertWindowForTest)
	if got := len(lark.messages()); got != 0 {
		t.Fatalf("recovery inside quiet hours: expected no messages, got %d", got)
	}
	// Both transitions of epHeal are on the record (events land regardless).
	if got := alertEventsOfKind(t, ts, "recovered"); len(got) != 1 {
		t.Fatalf("recovery inside quiet hours should still record its event, got %v", got)
	}

	// Window end: the summary names only the still-faulty endpoint.
	clock.Advance(7*time.Hour + 28*time.Minute) // 23:32 + 7h28m = 07:00
	msgs := waitForLarkMessages(t, lark, 1)
	if !strings.Contains(msgs[0], "stay-model") {
		t.Errorf("summary should name the still-faulty endpoint, got: %s", msgs[0])
	}
	if strings.Contains(msgs[0], "heal-model") {
		t.Errorf("summary must not mention the self-healed endpoint, got: %s", msgs[0])
	}

	// The self-healed endpoint's events honestly stay sent_ok=false — never
	// delivered — while the still-open endpoint's anchor is confirmed.
	waitForAlertEvents(t, ts, 4) // 2 down + 1 recovered + 1 quiet_summary
	for _, e := range alertEventsOfKind(t, ts, "recovered") {
		if e["sent_ok"].(bool) != false {
			t.Errorf("self-healed recovery event should stay sent_ok=false (never delivered), got %v", e)
		}
	}
}

// TestQuietHoursEmptySummaryNotSent: a quiet night whose only faults healed
// themselves (and no score_drop happened) ends in silence — the empty
// summary is never sent and never recorded.
func TestQuietHoursEmptySummaryNotSent(t *testing.T) {
	db := openTempDB(t)
	clock := scheduler.NewFakeClock(quietClockStart())
	ts := newAlertClockServer(t, db, clock)
	lark := newStubLarkServer(t)
	stubHub := newStubHubServer()
	defer stubHub.Close()

	configureWebhook(t, ts, lark, true)
	configureQuietHours(t, ts, true, quietTestStart, quietTestEnd)

	endpointID := createProbedEndpoint(t, ts, "Blip Hub", stubHub.URL, "blip-model")
	stubHub.SetMode("error_503")

	// Down and recovery both inside the window.
	runProbeRound(t, ts, endpointID)
	runProbeRound(t, ts, endpointID)
	clock.Advance(alertWindowForTest)
	stubHub.SetMode("success")
	runProbeRound(t, ts, endpointID)
	clock.Advance(alertWindowForTest)
	if got := len(lark.messages()); got != 0 {
		t.Fatalf("inside quiet hours: expected no messages, got %d", got)
	}

	// Window end: nothing still faulty, no deferred score_drop — silence.
	clock.Advance(8 * time.Hour) // well past 07:00
	// The boundary handler is asynchronous and the empty summary leaves no
	// trace by design; give it a bounded moment to (not) fire.
	time.Sleep(100 * time.Millisecond)
	if got := len(lark.messages()); got != 0 {
		t.Fatalf("empty summary must not be sent, got %d messages", got)
	}
	if got := quietSummaryEvents(t, ts); len(got) != 0 {
		t.Fatalf("empty summary must not be recorded, got %v", got)
	}
}

// TestQuietHoursDefersScoreDrop: a score_drop decided inside the window is
// recorded (sent_ok=false) but not sent; it rides the window-end summary,
// which confirms the deferred event.
func TestQuietHoursDefersScoreDrop(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "quiet-score.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	seedTestUser(t, db)
	stub := newEvalStubHub()
	defer stub.Close()

	// Campaign 1 runs at 22:00, outside the 23:00–07:00 window.
	clock := scheduler.NewFakeClock(time.Date(2026, time.July, 29, 22, 0, 0, 0, time.Local))
	ts := httptest.NewServer(server.New(db,
		server.WithRateLimits(server.RateLimits{}),
		server.WithSyncEval(),
		server.WithSyncDiscovery(),
		server.WithAlertClock(clock),
	))
	defer ts.Close()
	lark := newStubLarkServer(t)
	enableScoreDropAlerts(t, ts, lark)

	modelID := createEvalModel(t, ts.URL, stub.URL, "drop-model")
	suiteID := suiteIDByKey(t, ts.URL, "gsm8k")
	retireSuiteCases(t, db, suiteID)
	createRuleCase(t, ts.URL, suiteID, "DROP-A:请作答", "好的", nil)
	createRuleCase(t, ts.URL, suiteID, "DROP-B:请作答", "好的", nil)

	// Campaign 1 (aggregate 1.0): no baseline, no alert.
	run1 := triggerEval(t, ts.URL, suiteID, modelID)
	waitEvalDone(t, ts.URL, run1)
	waitCampaignStatus(t, ts.URL, campaignOfRun(t, ts.URL, run1), "done")
	if got := len(lark.messages()); got != 0 {
		t.Fatalf("first campaign: expected no alerts without a baseline, got %d", got)
	}

	// Enable quiet hours and step inside the window (22:00 → 23:30).
	configureQuietHours(t, ts, true, quietTestStart, quietTestEnd)
	clock.Advance(90 * time.Minute)

	// Campaign 2: the model turns bad (aggregate 0.0) — the 1.0 drop is
	// decided inside the window: the event lands with sent_ok=false and
	// nothing is sent.
	stub.markBad("drop-model", true)
	run2 := triggerEval(t, ts.URL, suiteID, modelID)
	waitEvalDone(t, ts.URL, run2)
	waitCampaignStatus(t, ts.URL, campaignOfRun(t, ts.URL, run2), "done")
	waitFor(t, "score_drop event recorded inside quiet hours", func() bool {
		return len(scoreDropEvents(t, ts)) == 1
	})
	if got := len(lark.messages()); got != 0 {
		t.Fatalf("score_drop inside quiet hours must not be sent, got %d messages", got)
	}
	if event := scoreDropEvents(t, ts)[0]; event["sent_ok"].(bool) != false {
		t.Errorf("deferred score_drop event: expected sent_ok false (delivery unconfirmed), got %v", event)
	}

	// Window end: the summary delivers the deferred drop and confirms it.
	clock.Advance(7*time.Hour + 30*time.Minute) // 07:00
	msgs := waitForLarkMessages(t, lark, 1)
	for _, want := range []string{"drop-model", "分数大跌", "静默"} {
		if !strings.Contains(msgs[0], want) {
			t.Errorf("quiet summary should contain %q, got: %s", want, msgs[0])
		}
	}
	// The summary carries the deferred score_drop's FULL frozen text (spec
	// axis (a)2: the delayed delivery must carry the substance, not a
	// first-line teaser) — pinned against the message persisted on the
	// deferred event, which is multi-line (per-suite and per-case lines).
	dropText := scoreDropEvents(t, ts)[0]["message"].(string)
	if !strings.Contains(dropText, "\n") {
		t.Fatalf("test premise: the frozen score_drop text should be multi-line, got: %s", dropText)
	}
	cards := lark.cards()
	if len(cards) != 1 {
		t.Fatalf("expected 1 card, got %d", len(cards))
	}
	if !strings.Contains(cards[0].Detail, dropText) {
		t.Errorf("quiet summary detail should carry the full frozen score_drop text\ndetail: %s\nwant text: %s", cards[0].Detail, dropText)
	}
	waitFor(t, "score_drop event confirmed by the summary", func() bool {
		events := scoreDropEvents(t, ts)
		return len(events) == 1 && events[0]["sent_ok"].(bool) == true
	})
	if got := quietSummaryEvents(t, ts); len(got) != 1 {
		t.Fatalf("expected 1 quiet_summary event, got %d", len(got))
	}
}

// TestQuietHoursLoginAlertExempt: the login brute-force alert is fully
// exempt from quiet hours — it fires immediately inside the window.
func TestQuietHoursLoginAlertExempt(t *testing.T) {
	db := openTempDB(t)
	lark := newStubLarkServer(t)
	clock := scheduler.NewFakeClock(quietClockStart())
	ts := loginAlertTestServer(t, db, server.LoginAlertPolicy{
		Threshold: 3,
		Window:    time.Minute,
		Cooldown:  time.Minute,
	}, server.WithAlertClock(clock))
	configureWebhook(t, ts, lark, true)
	configureQuietHours(t, ts, true, quietTestStart, quietTestEnd)

	for i := 0; i < 3; i++ {
		failedLogin(t, ts.URL, "admin", "")
	}
	// No clock advance: the alert arrives immediately, quiet hours or not.
	msgs := waitForLarkMessages(t, lark, 1)
	if !strings.Contains(msgs[0], "3") {
		t.Errorf("login alert should carry the failure count, got: %s", msgs[0])
	}
}

// TestQuietHoursCrossMidnightBoundaries pins the boundary semantics of the
// 23:00–07:00 window: 22:30 is outside (flush sends normally), 23:30 and
// 06:30 are inside (held), and an alert already delivered before the window
// is not repeated in the summary.
func TestQuietHoursCrossMidnightBoundaries(t *testing.T) {
	db := openTempDB(t)
	// 22:30 local: one hour before the window opens.
	clock := scheduler.NewFakeClock(time.Date(2026, time.July, 29, 22, 30, 0, 0, time.Local))
	ts := newAlertClockServer(t, db, clock)
	lark := newStubLarkServer(t)
	stubEarly := newStubHubServer()
	defer stubEarly.Close()
	stubLate := newStubHubServer()
	defer stubLate.Close()

	configureWebhook(t, ts, lark, true)
	configureQuietHours(t, ts, true, quietTestStart, quietTestEnd)

	// 22:30 is outside the window: the flush sends the aggregated card.
	epEarly := createProbedEndpoint(t, ts, "Early Hub", stubEarly.URL, "early-model")
	stubEarly.SetMode("error_503")
	runProbeRound(t, ts, epEarly)
	runProbeRound(t, ts, epEarly)
	clock.Advance(alertWindowForTest)
	msgs := waitForLarkMessages(t, lark, 1)
	if !strings.Contains(msgs[0], "early-model") {
		t.Errorf("outside the window the card should send normally, got: %s", msgs[0])
	}

	// 23:31 is inside: the late endpoint's flush is held.
	clock.Advance(time.Hour) // 23:31
	epLate := createProbedEndpoint(t, ts, "Late Hub", stubLate.URL, "late-model")
	stubLate.SetMode("error_503")
	runProbeRound(t, ts, epLate)
	runProbeRound(t, ts, epLate)
	clock.Advance(alertWindowForTest)
	if got := len(lark.messages()); got != 1 {
		t.Fatalf("23:31 is inside the window: expected still 1 message, got %d", got)
	}

	// 06:32 next morning is still inside: nothing yet.
	clock.Advance(7*time.Hour + time.Minute) // 06:32
	if got := len(lark.messages()); got != 1 {
		t.Fatalf("06:32 is inside the window: expected still 1 message, got %d", got)
	}

	// 07:00: the summary names only the late endpoint — the early endpoint's
	// alert was delivered before the window and is not repeated.
	clock.Advance(28 * time.Minute) // 07:00
	msgs = waitForLarkMessages(t, lark, 2)
	if !strings.Contains(msgs[1], "late-model") {
		t.Errorf("summary should name the endpoint faulted inside the window, got: %s", msgs[1])
	}
	if strings.Contains(msgs[1], "early-model") {
		t.Errorf("summary must not repeat the alert delivered before the window, got: %s", msgs[1])
	}
	if got := len(lark.messages()); got != 2 {
		t.Fatalf("expected exactly 2 messages total, got %d", got)
	}
}

// TestQuietHoursStartEqualsEndDisabled: start == end means "not enabled"
// even when the switch is on — behavior is identical to no quiet hours.
func TestQuietHoursStartEqualsEndDisabled(t *testing.T) {
	db := openTempDB(t)
	clock := scheduler.NewFakeClock(quietClockStart())
	ts := newAlertClockServer(t, db, clock)
	lark := newStubLarkServer(t)
	stubHub := newStubHubServer()
	defer stubHub.Close()

	configureWebhook(t, ts, lark, true)
	configureQuietHours(t, ts, true, 23, 23) // start == end: not enabled

	endpointID := createProbedEndpoint(t, ts, "Equal Hub", stubHub.URL, "equal-model")
	stubHub.SetMode("error_503")
	runProbeRound(t, ts, endpointID)
	runProbeRound(t, ts, endpointID)
	clock.Advance(alertWindowForTest)
	msgs := waitForLarkMessages(t, lark, 1)
	if !strings.Contains(msgs[0], "equal-model") {
		t.Errorf("start==end must behave as disabled (card sends normally), got: %s", msgs[0])
	}
	if got := quietSummaryEvents(t, ts); len(got) != 0 {
		t.Fatalf("start==end must not produce summaries, got %v", got)
	}
}

// TestQuietHoursSettingsRoundTrip covers the settings channel for the three
// new keys: defaults (disabled, 23→7), write/read-back, and hour validation.
func TestQuietHoursSettingsRoundTrip(t *testing.T) {
	db := openTempDB(t)
	ts := newTestAPIServer(t, db)

	get := func() map[string]interface{} {
		t.Helper()
		resp := doGet(t, ts.URL+"/api/settings")
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("get settings: expected 200, got %d", resp.StatusCode)
		}
		var env envelope
		if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
			t.Fatalf("decode settings: %v", err)
		}
		var settings map[string]interface{}
		if err := json.Unmarshal(env.Data, &settings); err != nil {
			t.Fatalf("unmarshal settings: %v", err)
		}
		return settings
	}

	// Defaults: disabled, 23:00–07:00.
	settings := get()
	if settings["quiet_hours_enabled"].(bool) != false {
		t.Errorf("default quiet_hours_enabled should be false, got %v", settings["quiet_hours_enabled"])
	}
	if int(settings["quiet_hours_start"].(float64)) != 23 {
		t.Errorf("default quiet_hours_start should be 23, got %v", settings["quiet_hours_start"])
	}
	if int(settings["quiet_hours_end"].(float64)) != 7 {
		t.Errorf("default quiet_hours_end should be 7, got %v", settings["quiet_hours_end"])
	}

	// Write and read back.
	resp := doPut(t, ts.URL+"/api/settings", map[string]interface{}{
		"quiet_hours_enabled": true,
		"quiet_hours_start":   22,
		"quiet_hours_end":     6,
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("put quiet hours: expected 200, got %d", resp.StatusCode)
	}
	settings = get()
	if settings["quiet_hours_enabled"].(bool) != true ||
		int(settings["quiet_hours_start"].(float64)) != 22 ||
		int(settings["quiet_hours_end"].(float64)) != 6 {
		t.Errorf("quiet hours should read back as written, got %v", settings)
	}

	// Hours must be 0–23.
	for _, patch := range []map[string]interface{}{
		{"quiet_hours_start": 24},
		{"quiet_hours_start": -1},
		{"quiet_hours_end": 24},
		{"quiet_hours_end": -1},
	} {
		resp := doPut(t, ts.URL+"/api/settings", patch)
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("patch %v: expected 400, got %d", patch, resp.StatusCode)
		}
	}
}

// TestQuietHoursSummaryIncludesOpenGroup: a vendor group alert opened inside
// the window is held like endpoint alerts, and the window-end summary lists
// the still-open group — folding its member endpoints into the group line
// rather than double-reporting them.
func TestQuietHoursSummaryIncludesOpenGroup(t *testing.T) {
	db := openTempDB(t)
	clock := scheduler.NewFakeClock(quietClockStart())
	ts := newAlertClockServer(t, db, clock)
	lark := newStubLarkServer(t)
	stubA := newStubHubServer()
	defer stubA.Close()
	stubB := newStubHubServer()
	defer stubB.Close()
	stubC := newStubHubServer()
	defer stubC.Close()
	stubD := newStubHubServer()
	defer stubD.Close()

	configureWebhook(t, ts, lark, true)
	configureQuietHours(t, ts, true, quietTestStart, quietTestEnd)

	epA := createGroupEndpoint(t, ts, "Quiet Group Hub A", stubA.URL, "gpt-q-a")
	epB := createGroupEndpoint(t, ts, "Quiet Group Hub B", stubB.URL, "gpt-q-b")
	_ = createGroupEndpoint(t, ts, "Quiet Group Hub C", stubC.URL, "gpt-q-c")
	_ = createGroupEndpoint(t, ts, "Quiet Group Hub D", stubD.URL, "gpt-q-d")

	// 2 of 4 enabled endpoints of family gpt go down inside the window: the
	// group opens, the flush holds — no messages.
	stubA.SetMode("error_503")
	stubB.SetMode("error_503")
	driveGroupEndpointDown(t, ts, epA)
	driveGroupEndpointDown(t, ts, epB)
	clock.Advance(alertWindowForTest)
	if got := len(lark.messages()); got != 0 {
		t.Fatalf("group alert inside quiet hours: expected no messages, got %d", got)
	}
	if got := alertEventsOfKind(t, ts, "group_down"); len(got) != 1 {
		t.Fatalf("group transition should still record its event, got %v", got)
	}

	// Window end: the summary lists the still-open group by family name and
	// does not double-report its member endpoints.
	clock.Advance(7*time.Hour + 29*time.Minute) // 23:31 + 7h29m = 07:00
	msgs := waitForLarkMessages(t, lark, 1)
	if !strings.Contains(msgs[0], "gpt") {
		t.Errorf("summary should name the still-open vendor group, got: %s", msgs[0])
	}
	if strings.Contains(msgs[0], "gpt-q-a") || strings.Contains(msgs[0], "gpt-q-b") {
		t.Errorf("summary must not double-report group members as endpoints, got: %s", msgs[0])
	}

	// The group anchor is confirmed by the summary; the absorbed member
	// events honestly stay sent_ok=false (never individually delivered, the
	// same honesty as the window's absorption semantics).
	waitFor(t, "group anchor confirmed by the summary", func() bool {
		events := alertEventsOfKind(t, ts, "group_down")
		return len(events) == 1 && events[0]["sent_ok"].(bool) == true
	})
	for _, e := range alertEventsOfKind(t, ts, "down") {
		if e["sent_ok"].(bool) != false {
			t.Errorf("absorbed member event should stay sent_ok=false, got %v", e)
		}
	}
	if got := quietSummaryEvents(t, ts); len(got) != 1 {
		t.Fatalf("expected 1 quiet_summary event, got %d", len(got))
	}
}

// TestQuietHoursWindowBornInsideYieldsToSummary (GH #67 MEDIUM-1): a
// transition at 06:59:30 starts its 60s window inside quiet hours but the
// window's fire time (07:00:30) lands on the loud side. The start-side gate
// holds the whole window for the 07:00 summary: one clock advance crossing
// both the quiet-end boundary and the window deadline delivers exactly one
// message — the summary — in either goroutine order, and the held flush
// never re-tells the story the summary confirmed.
func TestQuietHoursWindowBornInsideYieldsToSummary(t *testing.T) {
	db := openTempDB(t)
	// 06:59:30 local: inside the 23:00–07:00 window, 30s before it ends.
	clock := scheduler.NewFakeClock(time.Date(2026, time.July, 30, 6, 59, 30, 0, time.Local))
	ts := newAlertClockServer(t, db, clock)
	lark := newStubLarkServer(t)
	stubHub := newStubHubServer()
	defer stubHub.Close()

	configureWebhook(t, ts, lark, true)
	configureQuietHours(t, ts, true, quietTestStart, quietTestEnd)

	endpointID := createProbedEndpoint(t, ts, "Edge Hub", stubHub.URL, "edge-model")
	stubHub.SetMode("error_503")
	runProbeRound(t, ts, endpointID)
	runProbeRound(t, ts, endpointID) // down transition at 06:59:30
	if got := len(lark.messages()); got != 0 {
		t.Fatalf("inside quiet hours: expected no messages, got %d", got)
	}

	// One advance crossing both deadlines (quiet end 07:00:00, window fire
	// 07:00:30): exactly one message — the quiet summary — regardless of
	// which timer goroutine runs first.
	clock.Advance(61 * time.Second) // 07:00:31
	msgs := waitForLarkMessages(t, lark, 1)
	if !strings.Contains(msgs[0], "edge-model") {
		t.Errorf("quiet summary should name the still-faulty endpoint, got: %s", msgs[0])
	}
	waitForAlertEvents(t, ts, 2) // down + quiet_summary: summary settled
	// The held flush produces nothing observable by design; give any
	// (misbehaving) late send a bounded moment to appear.
	time.Sleep(100 * time.Millisecond)
	if got := len(lark.messages()); got != 1 {
		t.Fatalf("window born inside quiet must yield to the summary: expected exactly 1 message, got %d", got)
	}
	cards := lark.cards()
	if len(cards) != 1 || cards[0].Title != "静默摘要 · HubScope" {
		t.Fatalf("the single message must be the quiet summary card, got %v", cards)
	}
	if got := alertEventsOfKind(t, ts, "batch"); len(got) != 0 {
		t.Fatalf("held flush must not record a batch event, got %v", got)
	}
	if down := alertEventsOfKind(t, ts, "down"); len(down) != 1 || down[0]["sent_ok"].(bool) != true {
		t.Errorf("down anchor should be confirmed by the summary, got %v", down)
	}
}

// TestQuietHoursExactBoundarySingleReport (GH #67 MEDIUM-1): a transition at
// 06:59:00 makes the window deadline coincide exactly with the quiet end
// (both 07:00:00). The two timers firing in the same clock advance must not
// race into a double report: exactly one message, the summary.
func TestQuietHoursExactBoundarySingleReport(t *testing.T) {
	db := openTempDB(t)
	// 06:59:00 local: the 60s window's fire time is exactly 07:00:00.
	clock := scheduler.NewFakeClock(time.Date(2026, time.July, 30, 6, 59, 0, 0, time.Local))
	ts := newAlertClockServer(t, db, clock)
	lark := newStubLarkServer(t)
	stubHub := newStubHubServer()
	defer stubHub.Close()

	configureWebhook(t, ts, lark, true)
	configureQuietHours(t, ts, true, quietTestStart, quietTestEnd)

	endpointID := createProbedEndpoint(t, ts, "Exact Hub", stubHub.URL, "exact-model")
	stubHub.SetMode("error_503")
	runProbeRound(t, ts, endpointID)
	runProbeRound(t, ts, endpointID) // down transition at 06:59:00

	// Quiet-end timer and window timer share the 07:00:00 deadline and fire
	// in the same advance: either goroutine order, exactly one report.
	clock.Advance(61 * time.Second) // 07:00:01
	msgs := waitForLarkMessages(t, lark, 1)
	if !strings.Contains(msgs[0], "exact-model") {
		t.Errorf("quiet summary should name the still-faulty endpoint, got: %s", msgs[0])
	}
	waitForAlertEvents(t, ts, 2) // down + quiet_summary: summary settled
	// Any double report would arrive asynchronously; give it a bounded
	// moment to appear.
	time.Sleep(100 * time.Millisecond)
	if got := len(lark.messages()); got != 1 {
		t.Fatalf("exact-boundary advance must not double report: expected exactly 1 message, got %d", got)
	}
	cards := lark.cards()
	if len(cards) != 1 || cards[0].Title != "静默摘要 · HubScope" {
		t.Fatalf("the single message must be the quiet summary card, got %v", cards)
	}
}

// TestQuietHoursSummaryListsAbsorbedMembersOfDeliveredGroup pins a
// registered safe-direction duplicate (GH #67 HIGH-1 scenario A, accepted
// behavior): a group card delivered BEFORE the quiet window confirms the
// group anchor (sent_ok=true) but not the absorbed member events, whose
// individual stories the frozen group card told. At the quiet end those
// members are still open with unconfirmed anchors, so the summary lists
// them individually — a repeat of what the pre-quiet group card said, the
// safe direction (over-report, never swallow). The anchor's sent_ok cannot
// distinguish "absorbed member whose story the group card told" from
// "member who joined after the group opened"; GH #76 will make the summary
// coverage-set aware and fold these members.
func TestQuietHoursSummaryListsAbsorbedMembersOfDeliveredGroup(t *testing.T) {
	db := openTempDB(t)
	// 22:00 local: loud side — the group card flushes before the window.
	clock := scheduler.NewFakeClock(time.Date(2026, time.July, 29, 22, 0, 0, 0, time.Local))
	ts := newAlertClockServer(t, db, clock)
	lark := newStubLarkServer(t)
	stubA := newStubHubServer()
	defer stubA.Close()
	stubB := newStubHubServer()
	defer stubB.Close()
	stubC := newStubHubServer()
	defer stubC.Close()
	stubD := newStubHubServer()
	defer stubD.Close()

	configureWebhook(t, ts, lark, true)
	configureQuietHours(t, ts, true, quietTestStart, quietTestEnd)

	epA := createGroupEndpoint(t, ts, "Pre Group Hub A", stubA.URL, "gpt-pre-a")
	epB := createGroupEndpoint(t, ts, "Pre Group Hub B", stubB.URL, "gpt-pre-b")
	_ = createGroupEndpoint(t, ts, "Pre Group Hub C", stubC.URL, "gpt-pre-c")
	_ = createGroupEndpoint(t, ts, "Pre Group Hub D", stubD.URL, "gpt-pre-d")

	// 2 of 4 family endpoints go down at 22:00: the group opens and the
	// 22:01 flush delivers the group card — the group anchor is confirmed,
	// the absorbed member events stay sent_ok=false.
	stubA.SetMode("error_503")
	stubB.SetMode("error_503")
	driveGroupEndpointDown(t, ts, epA)
	driveGroupEndpointDown(t, ts, epB)
	clock.Advance(alertWindowForTest)
	msgs := waitForLarkMessages(t, lark, 1)
	if !strings.Contains(msgs[0], "gpt") {
		t.Errorf("pre-quiet flush should deliver the group card, got: %s", msgs[0])
	}

	// Into and out of the quiet window: 22:01 → 23:00 (boundary, no
	// summary) → 07:00 next morning.
	clock.Advance(59 * time.Minute) // 23:00
	clock.Advance(8 * time.Hour)    // 07:00
	msgs = waitForLarkMessages(t, lark, 2)

	// Registered duplicate (GH #76 will fold it): the summary lists the
	// absorbed members individually even though the pre-quiet group card
	// already told their story — the group anchor is confirmed so the group
	// line is skipped, while the member anchors stay unconfirmed.
	if !strings.Contains(msgs[1], "gpt-pre-a") || !strings.Contains(msgs[1], "gpt-pre-b") {
		t.Errorf("summary should list the still-open members individually (registered duplicate, GH #76), got: %s", msgs[1])
	}
	if got := quietSummaryEvents(t, ts); len(got) != 1 {
		t.Fatalf("expected 1 quiet_summary event, got %d", len(got))
	}
}

// TestQuietHoursSummaryRelistsAbsorbedMembersNextNight pins the second
// registered safe-direction duplicate (GH #67 HIGH-1 scenario B, accepted
// behavior): a group opens inside the quiet window, the end-of-window
// summary reports the group (confirming its anchor) and folds the members;
// the fault persists, and the NEXT quiet end the group line is skipped
// (anchor confirmed) while the member anchors are still unconfirmed — so
// the members reappear in the summary under their individual identities.
// GH #76 will make the summary coverage-set aware and fold them.
func TestQuietHoursSummaryRelistsAbsorbedMembersNextNight(t *testing.T) {
	db := openTempDB(t)
	clock := scheduler.NewFakeClock(quietClockStart()) // 23:30, inside quiet
	ts := newAlertClockServer(t, db, clock)
	lark := newStubLarkServer(t)
	stubA := newStubHubServer()
	defer stubA.Close()
	stubB := newStubHubServer()
	defer stubB.Close()
	stubC := newStubHubServer()
	defer stubC.Close()
	stubD := newStubHubServer()
	defer stubD.Close()

	configureWebhook(t, ts, lark, true)
	configureQuietHours(t, ts, true, quietTestStart, quietTestEnd)

	epA := createGroupEndpoint(t, ts, "Night Group Hub A", stubA.URL, "gpt-nite-a")
	epB := createGroupEndpoint(t, ts, "Night Group Hub B", stubB.URL, "gpt-nite-b")
	_ = createGroupEndpoint(t, ts, "Night Group Hub C", stubC.URL, "gpt-nite-c")
	_ = createGroupEndpoint(t, ts, "Night Group Hub D", stubD.URL, "gpt-nite-d")

	// The group opens inside the window; the flush holds.
	stubA.SetMode("error_503")
	stubB.SetMode("error_503")
	driveGroupEndpointDown(t, ts, epA)
	driveGroupEndpointDown(t, ts, epB)
	clock.Advance(alertWindowForTest)
	if got := len(lark.messages()); got != 0 {
		t.Fatalf("inside quiet hours: expected no messages, got %d", got)
	}

	// First quiet end: the summary reports the group (family line) and
	// folds its members; the group anchor is confirmed.
	clock.Advance(7*time.Hour + 29*time.Minute) // 23:31 + 7h29m = 07:00
	msgs := waitForLarkMessages(t, lark, 1)
	if !strings.Contains(msgs[0], "gpt") {
		t.Errorf("first summary should name the still-open vendor group, got: %s", msgs[0])
	}
	if strings.Contains(msgs[0], "gpt-nite-a") || strings.Contains(msgs[0], "gpt-nite-b") {
		t.Errorf("first summary should fold group members into the group line, got: %s", msgs[0])
	}
	waitFor(t, "group anchor confirmed by the first summary", func() bool {
		events := alertEventsOfKind(t, ts, "group_down")
		return len(events) == 1 && events[0]["sent_ok"].(bool) == true
	})

	// The fault persists through the day and the second quiet night.
	clock.Advance(16 * time.Hour) // 23:00: boundary, no summary (window entered)
	clock.Advance(8 * time.Hour)  // 07:00 next day
	msgs = waitForLarkMessages(t, lark, 2)

	// Registered duplicate (GH #76 will fold it): the group anchor is
	// confirmed, so the second summary skips the group line — and the
	// still-open members reappear under their individual identities.
	if !strings.Contains(msgs[1], "gpt-nite-a") || !strings.Contains(msgs[1], "gpt-nite-b") {
		t.Errorf("second summary should relist the still-open members individually (registered duplicate, GH #76), got: %s", msgs[1])
	}
	if got := quietSummaryEvents(t, ts); len(got) != 2 {
		t.Fatalf("expected 2 quiet_summary events, got %d", len(got))
	}
}
