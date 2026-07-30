package server_test

import (
	"database/sql"
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

// group_alerts_test.go covers the vendor (family) group alerts of spec 0017
// ticket 3 (GH #66): a group_down red card when ≥50% and ≥2 of a family's
// enabled endpoints are alerted, absorption of in-group endpoint transitions
// while the group is open, and a group_recovered green card (recovered /
// still-faulty sections) when the share falls below 50%. All tests are
// black-box at the W1 seam: manual probe rounds drive transitions, the
// injected fake clock flushes the 60s aggregation window, and assertions
// read only the Lark stub and GET /api/alerts.

// createGroupEndpoint registers one hub + model and keeps only its anthropic
// endpoint enabled, so the family's enabled-endpoint set (the group-alert
// denominator) is exactly the endpoints this helper returns — one per model.
// Model creation trials both chat protocols against the stub hub and would
// otherwise leave two enabled endpoints per model.
func createGroupEndpoint(t *testing.T, ts *httptest.Server, hubName, baseURL, modelID string) int64 {
	t.Helper()
	hubID := createHubForAlerts(t, ts, hubName, baseURL)

	modelResp := doPost(t, ts.URL+"/api/models", map[string]interface{}{
		"hub_id":   hubID,
		"model_id": modelID,
	})
	if modelResp.StatusCode != http.StatusCreated {
		t.Fatalf("create model %s: expected 201, got %d", modelID, modelResp.StatusCode)
	}
	var env envelope
	if err := json.NewDecoder(modelResp.Body).Decode(&env); err != nil {
		t.Fatalf("decode model: %v", err)
	}
	modelResp.Body.Close()
	var model map[string]interface{}
	if err := json.Unmarshal(env.Data, &model); err != nil {
		t.Fatalf("unmarshal model: %v", err)
	}

	var anthropicID int64
	for _, raw := range model["endpoints"].([]interface{}) {
		ep := raw.(map[string]interface{})
		id := int64(ep["id"].(float64))
		if ep["protocol"].(string) == "anthropic" {
			anthropicID = id
			continue
		}
		resp := doPatch(t, ts.URL+"/api/endpoints/"+itoa(id), map[string]interface{}{"enabled": false})
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("disable %s endpoint of %s: expected 200, got %d", ep["protocol"], modelID, resp.StatusCode)
		}
	}
	if anthropicID == 0 {
		t.Fatalf("model %s has no anthropic endpoint", modelID)
	}
	return anthropicID
}

// driveGroupEndpointDown pushes one endpoint across the down threshold (two
// rounds = four consecutive failures against an erroring hub).
func driveGroupEndpointDown(t *testing.T, ts *httptest.Server, endpointID int64) {
	t.Helper()
	runProbeRound(t, ts, endpointID)
	runProbeRound(t, ts, endpointID)
}

// alertEventOfKindForEndpoint returns the single event of the given kind for
// the given endpoint, failing the test if there is not exactly one.
func alertEventOfKindForEndpoint(t *testing.T, ts *httptest.Server, kind string, endpointID int64) map[string]interface{} {
	t.Helper()
	var matches []map[string]interface{}
	for _, e := range alertEventsOfKind(t, ts, kind) {
		if e["endpoint_id"] == nil {
			continue
		}
		if int64(e["endpoint_id"].(float64)) == endpointID {
			matches = append(matches, e)
		}
	}
	if len(matches) != 1 {
		t.Fatalf("expected exactly 1 %s event for endpoint %d, got %d", kind, endpointID, len(matches))
	}
	return matches[0]
}

// TestGroupAlertTriggersAtMajority drives 2 of a 4-endpoint family down: the
// flush sends exactly one red group card naming the vendor with per-hub
// detail, and a third in-group endpoint going down is absorbed — its event
// lands (sent_ok=false, never individually delivered) but no new message is
// sent. Also covers the API surface: the group_down event carries group_key
// and a NULL endpoint_id (GH #66 AC 1 + 5).
func TestGroupAlertTriggersAtMajority(t *testing.T) {
	db := openTempDB(t)
	clock := scheduler.NewFakeClock(time.Now())
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

	epA := createGroupEndpoint(t, ts, "Group Hub A", stubA.URL, "gpt-grp-a")
	epB := createGroupEndpoint(t, ts, "Group Hub B", stubB.URL, "gpt-grp-b")
	epC := createGroupEndpoint(t, ts, "Group Hub C", stubC.URL, "gpt-grp-c")
	_ = createGroupEndpoint(t, ts, "Group Hub D", stubD.URL, "gpt-grp-d")

	// 2 of 4 enabled endpoints of family gpt go down inside one window.
	stubA.SetMode("error_503")
	stubB.SetMode("error_503")
	driveGroupEndpointDown(t, ts, epA)
	driveGroupEndpointDown(t, ts, epB)

	// Decision time: the two down events and the group_down event are
	// persisted (sent_ok=false, "delivery unconfirmed"), nothing is sent.
	if got := len(lark.messages()); got != 0 {
		t.Fatalf("before window flush: expected no messages, got %d", got)
	}
	groupEvents := alertEventsOfKind(t, ts, "group_down")
	if len(groupEvents) != 1 {
		t.Fatalf("expected 1 group_down event at decision time, got %d", len(groupEvents))
	}
	if groupEvents[0]["group_key"] != "gpt" {
		t.Errorf("group_down event group_key: expected %q, got %v", "gpt", groupEvents[0]["group_key"])
	}
	if groupEvents[0]["endpoint_id"] != nil {
		t.Errorf("group_down event should be hub-less (NULL endpoint_id), got %v", groupEvents[0]["endpoint_id"])
	}
	if groupEvents[0]["sent_ok"].(bool) != false {
		t.Error("group_down event at decision time: expected sent_ok false (delivery unconfirmed)")
	}

	// Flush: exactly one red group card — the two in-group transitions were
	// absorbed into it rather than rendered as an endpoint card.
	clock.Advance(alertWindowForTest)
	waitForLarkMessages(t, lark, 1)
	waitForAlertEvents(t, ts, 4) // 2 down + 1 group_down + 1 batch
	if got := len(lark.messages()); got != 1 {
		t.Fatalf("2-of-4 family down: expected exactly 1 group card, got %d messages", got)
	}
	cards := lark.cards()
	if len(cards) != 1 {
		t.Fatalf("expected 1 card, got %d", len(cards))
	}
	group := cards[0]
	if group.Template != "red" {
		t.Errorf("group card template: expected red, got %q", group.Template)
	}
	if group.Title != "厂商组告警:gpt · HubScope" {
		t.Errorf("group card title should name the vendor, got %q", group.Title)
	}
	if group.Fields["厂商"] != "gpt" {
		t.Errorf("group card vendor field: got %q", group.Fields["厂商"])
	}
	if group.Fields["故障端点"] != "2/4 个" {
		t.Errorf("group card faulty-count field: got %q", group.Fields["故障端点"])
	}
	for _, want := range []string{"Group Hub A", "Group Hub B", "gpt-grp-a", "gpt-grp-b"} {
		if !strings.Contains(group.Detail, want) {
			t.Errorf("group card detail should contain %q, got: %s", want, group.Detail)
		}
	}
	if strings.Contains(group.Detail, "gpt-grp-c") || strings.Contains(group.Detail, "gpt-grp-d") {
		t.Errorf("group card detail must not list healthy members, got: %s", group.Detail)
	}
	if groupEvents = alertEventsOfKind(t, ts, "group_down"); groupEvents[0]["sent_ok"].(bool) != true {
		t.Error("group_down event sent_ok should be written back to true after the flush")
	}
	if got := alertEventsOfKind(t, ts, "batch"); len(got) != 1 {
		t.Errorf("expected 1 batch event for the group card, got %d", len(got))
	}

	// A third in-group endpoint goes down while the group is open: absorbed.
	// The event is recorded but no new message is sent, and the absorbed
	// event honestly stays sent_ok=false (never individually delivered).
	stubC.SetMode("error_503")
	driveGroupEndpointDown(t, ts, epC)
	clock.Advance(alertWindowForTest)
	waitForAlertEvents(t, ts, 5) // +1 absorbed down event; no new batch
	if got := len(lark.messages()); got != 1 {
		t.Fatalf("third in-group down must be absorbed: expected still 1 message, got %d", got)
	}
	if got := alertEventsOfKind(t, ts, "group_down"); len(got) != 1 {
		t.Errorf("absorbed transition must not re-trigger the group: expected still 1 group_down event, got %d", len(got))
	}
	absorbed := alertEventOfKindForEndpoint(t, ts, "down", epC)
	if absorbed["sent_ok"].(bool) != false {
		t.Error("absorbed down event should stay sent_ok=false (never individually delivered)")
	}
}

// TestGroupAlertRecoverySections opens a group with 3 of 4 endpoints down,
// then proves in-group recoveries are absorbed while the share stays ≥50%,
// and that falling below 50% sends one green group card with "已恢复 /
// 仍故障" sections (GH #66 AC 2).
func TestGroupAlertRecoverySections(t *testing.T) {
	db := openTempDB(t)
	clock := scheduler.NewFakeClock(time.Now())
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

	epA := createGroupEndpoint(t, ts, "Heal Hub A", stubA.URL, "gpt-heal-a")
	epB := createGroupEndpoint(t, ts, "Heal Hub B", stubB.URL, "gpt-heal-b")
	epC := createGroupEndpoint(t, ts, "Heal Hub C", stubC.URL, "gpt-heal-c")
	_ = createGroupEndpoint(t, ts, "Heal Hub D", stubD.URL, "gpt-heal-d")

	// 3 of 4 down: the group opens at the second transition, the third is
	// absorbed into the same flush.
	stubA.SetMode("error_503")
	stubB.SetMode("error_503")
	stubC.SetMode("error_503")
	driveGroupEndpointDown(t, ts, epA)
	driveGroupEndpointDown(t, ts, epB)
	driveGroupEndpointDown(t, ts, epC)
	clock.Advance(alertWindowForTest)
	waitForLarkMessages(t, lark, 1)
	waitForAlertEvents(t, ts, 5) // 3 down + 1 group_down + 1 batch
	if got := len(lark.messages()); got != 1 {
		t.Fatalf("3-of-4 family down: expected exactly 1 group card, got %d messages", got)
	}

	// One recovery: 2/4 still alerted = 50% → the group stays open and the
	// recovery is absorbed (no message, event recorded).
	stubA.SetMode("success")
	runProbeRound(t, ts, epA)
	clock.Advance(alertWindowForTest)
	waitForAlertEvents(t, ts, 6) // +1 absorbed recovered event
	if got := len(lark.messages()); got != 1 {
		t.Fatalf("in-group recovery at 50%% must be absorbed: expected still 1 message, got %d", got)
	}

	// A second recovery drops the share to 1/4 < 50%: one green group
	// recovery card with recovered / still-faulty sections.
	stubB.SetMode("success")
	runProbeRound(t, ts, epB)
	clock.Advance(alertWindowForTest)
	waitForLarkMessages(t, lark, 2)
	waitForAlertEvents(t, ts, 9) // +1 recovered +1 group_recovered +1 batch
	if got := len(lark.messages()); got != 2 {
		t.Fatalf("share falling below 50%%: expected exactly 2 messages, got %d", got)
	}
	cards := lark.cards()
	if len(cards) != 2 {
		t.Fatalf("expected 2 cards, got %d", len(cards))
	}
	recovered := cards[1]
	if recovered.Template != "green" {
		t.Errorf("group recovery card template: expected green, got %q", recovered.Template)
	}
	if recovered.Title != "厂商组恢复:gpt · HubScope" {
		t.Errorf("group recovery card title should name the vendor, got %q", recovered.Title)
	}
	for _, want := range []string{"已恢复", "gpt-heal-a", "gpt-heal-b", "仍故障", "gpt-heal-c"} {
		if !strings.Contains(recovered.Detail, want) {
			t.Errorf("group recovery detail should contain %q, got: %s", want, recovered.Detail)
		}
	}
	if strings.Contains(recovered.Detail, "gpt-heal-d") {
		t.Errorf("never-faulty member must not appear in the recovery card, got: %s", recovered.Detail)
	}
	groupRecovered := alertEventsOfKind(t, ts, "group_recovered")
	if len(groupRecovered) != 1 {
		t.Fatalf("expected 1 group_recovered event, got %d", len(groupRecovered))
	}
	if groupRecovered[0]["group_key"] != "gpt" {
		t.Errorf("group_recovered event group_key: expected %q, got %v", "gpt", groupRecovered[0]["group_key"])
	}
	if groupRecovered[0]["sent_ok"].(bool) != true {
		t.Error("group_recovered event sent_ok should be written back to true after the flush")
	}
}

// TestGroupAlertThresholds pins the trigger boundaries (GH #66 AC 3): a
// single-endpoint family never triggers, one down in a two-endpoint family
// does not trigger (the ≥2 floor), and an endpoint alert sent before the
// group triggered is left alone (sent_ok stays true, no retraction).
func TestGroupAlertThresholds(t *testing.T) {
	db := openTempDB(t)
	clock := scheduler.NewFakeClock(time.Now())
	ts := newAlertClockServer(t, db, clock)
	lark := newStubLarkServer(t)
	stubSolo := newStubHubServer()
	defer stubSolo.Close()
	stubA := newStubHubServer()
	defer stubA.Close()
	stubB := newStubHubServer()
	defer stubB.Close()

	configureWebhook(t, ts, lark, true)

	// Single-endpoint family (llama): 1/1 alerted is 100% but below the
	// ≥2 floor — the alert stays an ordinary endpoint card.
	epSolo := createGroupEndpoint(t, ts, "Solo Hub", stubSolo.URL, "llama-solo-x")
	stubSolo.SetMode("error_503")
	driveGroupEndpointDown(t, ts, epSolo)
	clock.Advance(alertWindowForTest)
	waitForLarkMessages(t, lark, 1)
	waitForAlertEvents(t, ts, 2) // 1 down + 1 batch
	cards := lark.cards()
	if len(cards) != 1 || cards[0].Title != "端点告警 · HubScope" {
		t.Fatalf("single-endpoint family must send an endpoint card, got %v", cards)
	}
	if got := alertEventsOfKind(t, ts, "group_down"); len(got) != 0 {
		t.Fatalf("single-endpoint family must not trigger a group alert, got %v", got)
	}

	// Two-endpoint family (qwen): the first down is 1/2 = 50% but below
	// the ≥2 floor — ordinary endpoint card, sent before any group exists.
	epA := createGroupEndpoint(t, ts, "Pair Hub A", stubA.URL, "qwen-pair-a")
	epB := createGroupEndpoint(t, ts, "Pair Hub B", stubB.URL, "qwen-pair-b")
	stubA.SetMode("error_503")
	driveGroupEndpointDown(t, ts, epA)
	clock.Advance(alertWindowForTest)
	waitForLarkMessages(t, lark, 2)
	waitForAlertEvents(t, ts, 4) // +1 down +1 batch
	cards = lark.cards()
	if len(cards) != 2 || cards[1].Title != "端点告警 · HubScope" {
		t.Fatalf("1-of-2 family down must send an endpoint card, got %v", cards)
	}
	if got := alertEventsOfKind(t, ts, "group_down"); len(got) != 0 {
		t.Fatalf("1-of-2 family down must not trigger a group alert, got %v", got)
	}

	// The second down reaches 2/2 ≥ 50% and ≥2: the group opens. The first
	// endpoint's already-sent alert is not retracted (sent_ok stays true),
	// and the still-faulty first endpoint appears in the group card detail.
	stubB.SetMode("error_503")
	driveGroupEndpointDown(t, ts, epB)
	clock.Advance(alertWindowForTest)
	waitForLarkMessages(t, lark, 3)
	waitForAlertEvents(t, ts, 7) // +1 down +1 group_down +1 batch
	cards = lark.cards()
	if len(cards) != 3 {
		t.Fatalf("expected 3 cards, got %d", len(cards))
	}
	group := cards[2]
	if group.Title != "厂商组告警:qwen · HubScope" {
		t.Errorf("group card title should name the vendor, got %q", group.Title)
	}
	for _, want := range []string{"qwen-pair-a", "qwen-pair-b"} {
		if !strings.Contains(group.Detail, want) {
			t.Errorf("group card detail should list faulty member %q, got: %s", want, group.Detail)
		}
	}
	first := alertEventOfKindForEndpoint(t, ts, "down", epA)
	if first["sent_ok"].(bool) != true {
		t.Error("endpoint alert sent before the group triggered must not be retracted (sent_ok stays true)")
	}
	if got := alertEventsOfKind(t, ts, "group_down"); len(got) != 1 || got[0]["group_key"] != "qwen" {
		t.Errorf("expected exactly 1 group_down event with group_key qwen, got %v", got)
	}
}

// TestGroupAlertCoexistsWithEndpointCards proves group cards and out-of-group
// endpoint cards share one window flush, group cards first (GH #66 AC 4).
func TestGroupAlertCoexistsWithEndpointCards(t *testing.T) {
	db := openTempDB(t)
	clock := scheduler.NewFakeClock(time.Now())
	ts := newAlertClockServer(t, db, clock)
	lark := newStubLarkServer(t)
	stubA := newStubHubServer()
	defer stubA.Close()
	stubB := newStubHubServer()
	defer stubB.Close()
	stubC := newStubHubServer()
	defer stubC.Close()

	configureWebhook(t, ts, lark, true)

	epA := createGroupEndpoint(t, ts, "Mix Hub A", stubA.URL, "gpt-mix-a")
	epB := createGroupEndpoint(t, ts, "Mix Hub B", stubB.URL, "gpt-mix-b")
	epC := createGroupEndpoint(t, ts, "Mix Hub C", stubC.URL, "llama-mix-c")

	// Same window: the gpt pair opens a group; the single llama endpoint is
	// out of any group and flushes as an ordinary endpoint card.
	stubA.SetMode("error_503")
	stubB.SetMode("error_503")
	stubC.SetMode("error_503")
	driveGroupEndpointDown(t, ts, epA)
	driveGroupEndpointDown(t, ts, epB)
	driveGroupEndpointDown(t, ts, epC)
	clock.Advance(alertWindowForTest)
	waitForLarkMessages(t, lark, 2)
	waitForAlertEvents(t, ts, 6) // 3 down + 1 group_down + 2 batch
	if got := len(lark.messages()); got != 2 {
		t.Fatalf("expected exactly 2 cards (group + endpoint), got %d", got)
	}
	cards := lark.cards()
	groupCard, endpointCard := cards[0], cards[1]
	if groupCard.Title != "厂商组告警:gpt · HubScope" {
		t.Errorf("first card should be the group card (group cards lead the flush), got %q", groupCard.Title)
	}
	if strings.Contains(groupCard.Detail, "llama-mix-c") {
		t.Errorf("group card must not list out-of-group endpoints, got: %s", groupCard.Detail)
	}
	if endpointCard.Title != "端点告警 · HubScope" {
		t.Errorf("second card should be the endpoint card, got %q", endpointCard.Title)
	}
	if !strings.Contains(endpointCard.Detail, "llama-mix-c") {
		t.Errorf("endpoint card should contain the out-of-group model, got: %s", endpointCard.Detail)
	}
	if strings.Contains(endpointCard.Detail, "gpt-mix-a") || strings.Contains(endpointCard.Detail, "gpt-mix-b") {
		t.Errorf("endpoint card must not list absorbed in-group endpoints, got: %s", endpointCard.Detail)
	}
}

// TestGroupAlertRestartRebuild simulates a service restart while a group is
// open: the fresh process rebuilds the open state from the persisted
// group_down event — it neither re-sends the group alert nor loses
// absorption, and the group still closes with a recovery card once the
// share falls below 50% (GH #66 AC 6).
func TestGroupAlertRestartRebuild(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "group-restart.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	seedTestUser(t, db)

	lark := newStubLarkServer(t)
	stubA := newStubHubServer()
	defer stubA.Close()
	stubB := newStubHubServer()
	defer stubB.Close()
	stubC := newStubHubServer()
	defer stubC.Close()
	stubD := newStubHubServer()
	defer stubD.Close()

	// First process: 2 of 4 endpoints of family gpt go down and the group
	// card flushes.
	clock1 := scheduler.NewFakeClock(time.Now())
	ts1 := httptest.NewServer(server.New(db,
		server.WithRateLimits(server.RateLimits{}),
		server.WithSyncDiscovery(),
		server.WithAlertClock(clock1),
	))
	configureWebhook(t, ts1, lark, true)
	epA := createGroupEndpoint(t, ts1, "Restart Hub A", stubA.URL, "gpt-rst-a")
	epB := createGroupEndpoint(t, ts1, "Restart Hub B", stubB.URL, "gpt-rst-b")
	epC := createGroupEndpoint(t, ts1, "Restart Hub C", stubC.URL, "gpt-rst-c")
	_ = createGroupEndpoint(t, ts1, "Restart Hub D", stubD.URL, "gpt-rst-d")
	stubA.SetMode("error_503")
	stubB.SetMode("error_503")
	driveGroupEndpointDown(t, ts1, epA)
	driveGroupEndpointDown(t, ts1, epB)
	clock1.Advance(alertWindowForTest)
	waitForLarkMessages(t, lark, 1)
	waitForAlertEvents(t, ts1, 4) // 2 down + 1 group_down + 1 batch
	ts1.Close()

	// "Restart": a fresh evaluator over the same database. A third in-group
	// endpoint going down must be absorbed (the group is still open from
	// the persisted event) — no new message, no duplicate group_down.
	clock2 := scheduler.NewFakeClock(time.Now())
	ts2 := httptest.NewServer(server.New(db,
		server.WithRateLimits(server.RateLimits{}),
		server.WithSyncDiscovery(),
		server.WithAlertClock(clock2),
	))
	defer ts2.Close()
	stubC.SetMode("error_503")
	driveGroupEndpointDown(t, ts2, epC)
	clock2.Advance(alertWindowForTest)
	waitForAlertEvents(t, ts2, 5) // +1 absorbed down event
	if got := len(lark.messages()); got != 1 {
		t.Fatalf("after restart: in-group down must be absorbed, expected still 1 message, got %d", got)
	}
	if got := alertEventsOfKind(t, ts2, "group_down"); len(got) != 1 {
		t.Fatalf("after restart: expected still 1 group_down event (no duplicate), got %d", len(got))
	}

	// The rebuilt group still closes: two recoveries drop the share to 1/4
	// and exactly one group recovery card goes out.
	stubA.SetMode("success")
	stubB.SetMode("success")
	runProbeRound(t, ts2, epA)
	clock2.Advance(alertWindowForTest)
	runProbeRound(t, ts2, epB)
	clock2.Advance(alertWindowForTest)
	waitForLarkMessages(t, lark, 2)
	cards := lark.cards()
	if len(cards) != 2 {
		t.Fatalf("expected 2 cards, got %d", len(cards))
	}
	if cards[1].Template != "green" || cards[1].Title != "厂商组恢复:gpt · HubScope" {
		t.Errorf("second card should be the green group recovery card, got template %q title %q",
			cards[1].Template, cards[1].Title)
	}
	if !strings.Contains(cards[1].Detail, "gpt-rst-c") {
		t.Errorf("still-faulty section should list the endpoint that went down after the restart, got: %s", cards[1].Detail)
	}
	if got := alertEventsOfKind(t, ts2, "group_recovered"); len(got) != 1 {
		t.Errorf("expected 1 group_recovered event, got %d", len(got))
	}
}

// TestGroupAlertRecoveryAfterCloseRenders is the regression test for check
// GH #66 HIGH-1 counter-example 1: a group opens and closes inside one
// window, and an endpoint recovers AFTER the close. That recovery belongs
// to no group card (the green card's snapshot froze while the endpoint was
// still faulty) and must render as an ordinary endpoint recovery card —
// family-wide flush absorption would swallow it, leaving the endpoint
// forever "仍故障" with its recovery never reported. The ordered-consumption
// coverage replay (MEDIUM-1) additionally renders A's pre-trigger down as
// an endpoint card: the green card closed A's fault story, so the replay
// revoked A's coverage — a deliberate over-report (safe direction).
func TestGroupAlertRecoveryAfterCloseRenders(t *testing.T) {
	db := openTempDB(t)
	clock := scheduler.NewFakeClock(time.Now())
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

	epA := createGroupEndpoint(t, ts, "Flap Hub A", stubA.URL, "gpt-flap-a")
	epB := createGroupEndpoint(t, ts, "Flap Hub B", stubB.URL, "gpt-flap-b")
	_ = createGroupEndpoint(t, ts, "Flap Hub C", stubC.URL, "gpt-flap-c")
	_ = createGroupEndpoint(t, ts, "Flap Hub D", stubD.URL, "gpt-flap-d")

	// One window, four transitions: A down (buffered before the trigger),
	// B down (opens the group at 2/4), A recovers (closes it at 1/4), B
	// recovers (group already closed — its decision-time absorbed flag is
	// false and no group card mentions this recovery).
	stubA.SetMode("error_503")
	stubB.SetMode("error_503")
	driveGroupEndpointDown(t, ts, epA)
	driveGroupEndpointDown(t, ts, epB)
	stubA.SetMode("success")
	runProbeRound(t, ts, epA)
	stubB.SetMode("success")
	runProbeRound(t, ts, epB)
	if got := len(lark.messages()); got != 0 {
		t.Fatalf("before window flush: expected no messages, got %d", got)
	}

	clock.Advance(alertWindowForTest)
	waitForLarkMessages(t, lark, 4)
	waitForAlertEvents(t, ts, 10) // 2 down + 2 recovered + 2 group + 4 batch
	cards := lark.cards()
	if len(cards) != 4 {
		t.Fatalf("open+close in one window plus post-close transitions: expected 4 cards, got %d", len(cards))
	}

	// Card 1: the red group card, frozen at the trigger — A and B both
	// alerted at that moment.
	groupDown := cards[0]
	if groupDown.Title != "厂商组告警:gpt · HubScope" || groupDown.Template != "red" {
		t.Errorf("first card should be the red group card, got template %q title %q", groupDown.Template, groupDown.Title)
	}
	for _, want := range []string{"gpt-flap-a", "gpt-flap-b"} {
		if !strings.Contains(groupDown.Detail, want) {
			t.Errorf("group card detail should contain %q, got: %s", want, groupDown.Detail)
		}
	}

	// Card 2: the green group card, frozen at the close — A already
	// recovered, B still faulty at that moment.
	groupRecovered := cards[1]
	if groupRecovered.Title != "厂商组恢复:gpt · HubScope" || groupRecovered.Template != "green" {
		t.Errorf("second card should be the green group card, got template %q title %q", groupRecovered.Template, groupRecovered.Title)
	}
	if !strings.Contains(groupRecovered.Detail, "已恢复") || !strings.Contains(groupRecovered.Detail, "gpt-flap-a") {
		t.Errorf("group recovery card should list A as recovered, got: %s", groupRecovered.Detail)
	}
	if !strings.Contains(groupRecovered.Detail, "仍故障") || !strings.Contains(groupRecovered.Detail, "gpt-flap-b") {
		t.Errorf("group recovery card should list B as still faulty (frozen at the close), got: %s", groupRecovered.Detail)
	}

	// Card 3: A's pre-trigger down renders as an endpoint down card — the
	// green card closed A's fault story, so the ordered-consumption replay
	// revoked A's coverage (deliberate over-report, safe direction).
	endpointDown := cards[2]
	if endpointDown.Title != "端点告警 · HubScope" || endpointDown.Template != "red" {
		t.Errorf("third card should be the endpoint down card, got template %q title %q",
			endpointDown.Template, endpointDown.Title)
	}
	if !strings.Contains(endpointDown.Detail, "gpt-flap-a") {
		t.Errorf("endpoint down card should name A's model, got: %s", endpointDown.Detail)
	}
	if strings.Contains(endpointDown.Detail, "gpt-flap-b") {
		t.Errorf("B's down belongs to the group card (decision-time absorbed), not to the endpoint card, got: %s", endpointDown.Detail)
	}

	// Card 4: B's post-close recovery MUST render as an endpoint recovery
	// card — the whole point of the HIGH-1 regression.
	endpointRecovered := cards[3]
	if endpointRecovered.Title != "端点恢复 · HubScope" || endpointRecovered.Template != "green" {
		t.Errorf("fourth card should be the endpoint recovery card, got template %q title %q",
			endpointRecovered.Template, endpointRecovered.Title)
	}
	if !strings.Contains(endpointRecovered.Detail, "gpt-flap-b") {
		t.Errorf("endpoint recovery card should name B's model, got: %s", endpointRecovered.Detail)
	}
	if strings.Contains(endpointRecovered.Detail, "gpt-flap-a") {
		t.Errorf("A's recovery belongs to the group card (已恢复 section), not to the endpoint card, got: %s", endpointRecovered.Detail)
	}

	// Every transition's event landed; the rendered ones got their sent_ok
	// written back, the absorbed ones stay false (never individually
	// delivered).
	if got := alertEventOfKindForEndpoint(t, ts, "down", epA); got["sent_ok"].(bool) != true {
		t.Error("A's pre-trigger down was rendered (coverage revoked) — its event must be sent_ok=true")
	}
	recoveredB := alertEventOfKindForEndpoint(t, ts, "recovered", epB)
	if recoveredB["sent_ok"].(bool) != true {
		t.Error("B's post-close recovery was rendered — its event must be sent_ok=true")
	}
	recoveredA := alertEventOfKindForEndpoint(t, ts, "recovered", epA)
	if recoveredA["sent_ok"].(bool) != false {
		t.Error("A's recovery was absorbed into the group card — its event stays sent_ok=false")
	}
}

// TestGroupAlertRedownAfterCloseRenders is the regression test for check
// GH #66 MEDIUM-1: a member whose recovery the green card told goes down
// AGAIN inside the same window. The plain-union coverage set would still
// contain the member (group_recovered never subtracted) and swallow the
// re-down — the reader last saw "A 已恢复" and would never learn A failed
// again. The ordered-consumption replay revokes A's coverage when the green
// card closes its story, so the re-down renders as an endpoint down card.
func TestGroupAlertRedownAfterCloseRenders(t *testing.T) {
	db := openTempDB(t)
	clock := scheduler.NewFakeClock(time.Now())
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

	epA := createGroupEndpoint(t, ts, "Redown Hub A", stubA.URL, "gpt-redown-a")
	epB := createGroupEndpoint(t, ts, "Redown Hub B", stubB.URL, "gpt-redown-b")
	_ = createGroupEndpoint(t, ts, "Redown Hub C", stubC.URL, "gpt-redown-c")
	_ = createGroupEndpoint(t, ts, "Redown Hub D", stubD.URL, "gpt-redown-d")

	// One window, five transitions: A down, B down (opens the group at
	// 2/4), A recovers (closes it at 1/4 — the green card tells A's
	// recovery), B recovers (post-close, ordinary endpoint card), A down
	// AGAIN (group closed, 1/4, no re-trigger — a brand-new story belonging
	// to no group card).
	stubA.SetMode("error_503")
	stubB.SetMode("error_503")
	driveGroupEndpointDown(t, ts, epA)
	driveGroupEndpointDown(t, ts, epB)
	stubA.SetMode("success")
	runProbeRound(t, ts, epA)
	stubB.SetMode("success")
	runProbeRound(t, ts, epB)
	// Probes are stamped with wall-clock time at second precision and
	// CountConsecutiveFailures counts failures strictly after the last
	// success — the fake clock drives only the window, not probe
	// timestamps. Let the next second roll over so A's re-down failures
	// outrank the recovery round's successes.
	time.Sleep(1100 * time.Millisecond)
	stubA.SetMode("error_503")
	driveGroupEndpointDown(t, ts, epA)
	if got := len(lark.messages()); got != 0 {
		t.Fatalf("before window flush: expected no messages, got %d", got)
	}

	clock.Advance(alertWindowForTest)
	waitForLarkMessages(t, lark, 4)
	waitForAlertEvents(t, ts, 11) // 3 down + 2 recovered + 2 group + 4 batch
	cards := lark.cards()
	if len(cards) != 4 {
		t.Fatalf("expected 4 cards (2 group + endpoint down + endpoint recovery), got %d", len(cards))
	}

	groupDown, groupRecovered := cards[0], cards[1]
	if groupDown.Title != "厂商组告警:gpt · HubScope" {
		t.Errorf("first card should be the red group card, got %q", groupDown.Title)
	}
	if groupRecovered.Title != "厂商组恢复:gpt · HubScope" {
		t.Errorf("second card should be the green group card, got %q", groupRecovered.Title)
	}
	if !strings.Contains(groupRecovered.Detail, "已恢复") || !strings.Contains(groupRecovered.Detail, "gpt-redown-a") {
		t.Errorf("green card should tell A's recovery, got: %s", groupRecovered.Detail)
	}

	// Card 3: the endpoint down card carries BOTH of A's down transitions
	// (the pre-trigger one whose coverage the green card revoked, and the
	// post-close re-down) — A's renewed failure is fully reported.
	endpointDown := cards[2]
	if endpointDown.Title != "端点告警 · HubScope" || endpointDown.Template != "red" {
		t.Errorf("third card should be the endpoint down card, got template %q title %q",
			endpointDown.Template, endpointDown.Title)
	}
	if occurrences := strings.Count(endpointDown.Detail, "gpt-redown-a("); occurrences != 2 {
		t.Errorf("endpoint down card should list A twice (two down transitions), got %d occurrences in: %s",
			occurrences, endpointDown.Detail)
	}
	if strings.Contains(endpointDown.Detail, "gpt-redown-b") {
		t.Errorf("B's down belongs to the group card (decision-time absorbed), not to the endpoint card, got: %s",
			endpointDown.Detail)
	}

	// Card 4: B's post-close recovery as an ordinary endpoint card.
	endpointRecovered := cards[3]
	if endpointRecovered.Title != "端点恢复 · HubScope" || !strings.Contains(endpointRecovered.Detail, "gpt-redown-b") {
		t.Errorf("fourth card should be the endpoint recovery card naming B, got title %q detail %q",
			endpointRecovered.Title, endpointRecovered.Detail)
	}

	// A produced two down events; both were rendered (sent_ok=true). A's
	// recovery stayed absorbed into the green card (sent_ok=false).
	aDownSent, aDownTotal := 0, 0
	for _, e := range alertEventsOfKind(t, ts, "down") {
		if e["endpoint_id"] == nil || int64(e["endpoint_id"].(float64)) != epA {
			continue
		}
		aDownTotal++
		if e["sent_ok"].(bool) {
			aDownSent++
		}
	}
	if aDownTotal != 2 || aDownSent != 2 {
		t.Errorf("expected 2 down events for A, both rendered (sent_ok=true), got total=%d sent=%d", aDownTotal, aDownSent)
	}
	if got := alertEventOfKindForEndpoint(t, ts, "recovered", epA); got["sent_ok"].(bool) != false {
		t.Error("A's recovery was absorbed into the group card — its event stays sent_ok=false")
	}
}

// TestGroupAlertHealedBeforeTriggerRenders is the regression test for check
// GH #66 HIGH-1 counter-example 2: an endpoint goes down and recovers
// BEFORE its group opens inside the same window. Neither transition appears
// in the group card's frozen faulty snapshot, so both must render as
// ordinary endpoint cards — family-wide flush absorption would swallow the
// complete fault-and-heal story.
func TestGroupAlertHealedBeforeTriggerRenders(t *testing.T) {
	db := openTempDB(t)
	clock := scheduler.NewFakeClock(time.Now())
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

	epA := createGroupEndpoint(t, ts, "Heal Hub A", stubA.URL, "gpt-seq-a")
	epB := createGroupEndpoint(t, ts, "Heal Hub B", stubB.URL, "gpt-seq-b")
	epC := createGroupEndpoint(t, ts, "Heal Hub C", stubC.URL, "gpt-seq-c")
	_ = createGroupEndpoint(t, ts, "Heal Hub D", stubD.URL, "gpt-seq-d")

	// One window: A goes down and heals on its own (both transitions
	// buffered, group never open for them), then B and C go down and open
	// the group — its frozen faulty snapshot is {B, C} and says nothing
	// about A.
	stubA.SetMode("error_503")
	driveGroupEndpointDown(t, ts, epA)
	stubA.SetMode("success")
	runProbeRound(t, ts, epA)
	stubB.SetMode("error_503")
	stubC.SetMode("error_503")
	driveGroupEndpointDown(t, ts, epB)
	driveGroupEndpointDown(t, ts, epC)
	if got := len(lark.messages()); got != 0 {
		t.Fatalf("before window flush: expected no messages, got %d", got)
	}

	clock.Advance(alertWindowForTest)
	waitForLarkMessages(t, lark, 3)
	waitForAlertEvents(t, ts, 8) // 3 down + 1 recovered + 1 group_down + 3 batch
	cards := lark.cards()
	if len(cards) != 3 {
		t.Fatalf("expected 3 cards (group + endpoint down + endpoint recovery), got %d", len(cards))
	}

	// Card 1: the red group card covers B and C only — A healed before the
	// trigger and must not appear.
	groupDown := cards[0]
	if groupDown.Title != "厂商组告警:gpt · HubScope" {
		t.Errorf("first card should be the group card, got %q", groupDown.Title)
	}
	for _, want := range []string{"gpt-seq-b", "gpt-seq-c"} {
		if !strings.Contains(groupDown.Detail, want) {
			t.Errorf("group card detail should contain %q, got: %s", want, groupDown.Detail)
		}
	}
	if strings.Contains(groupDown.Detail, "gpt-seq-a") {
		t.Errorf("A healed before the trigger and must not appear in the group card, got: %s", groupDown.Detail)
	}

	// Cards 2 and 3: A's complete fault-and-heal story, as ordinary
	// endpoint cards.
	downCard, recoveredCard := cards[1], cards[2]
	if downCard.Title != "端点告警 · HubScope" || !strings.Contains(downCard.Detail, "gpt-seq-a") {
		t.Errorf("second card should be the endpoint down card naming A, got title %q detail %q", downCard.Title, downCard.Detail)
	}
	if strings.Contains(downCard.Detail, "gpt-seq-b") || strings.Contains(downCard.Detail, "gpt-seq-c") {
		t.Errorf("endpoint down card must not list absorbed group members, got: %s", downCard.Detail)
	}
	if recoveredCard.Title != "端点恢复 · HubScope" || !strings.Contains(recoveredCard.Detail, "gpt-seq-a") {
		t.Errorf("third card should be the endpoint recovery card naming A, got title %q detail %q",
			recoveredCard.Title, recoveredCard.Detail)
	}

	// A's transitions were rendered: both events get their sent_ok written
	// back (they were individually delivered, unlike absorbed ones).
	if got := alertEventOfKindForEndpoint(t, ts, "down", epA); got["sent_ok"].(bool) != true {
		t.Error("A's pre-trigger down was rendered — its event must be sent_ok=true")
	}
	if got := alertEventOfKindForEndpoint(t, ts, "recovered", epA); got["sent_ok"].(bool) != true {
		t.Error("A's pre-trigger recovery was rendered — its event must be sent_ok=true")
	}
}

// TestGroupAlertSchemaMigration proves the group_key column migration is
// lossless for pre-ticket databases: an alert_events table created without
// the column keeps its rows through store.Open, the column arrives
// automatically, and group alerts work on the upgraded database
// (GH #66 AC 5).
func TestGroupAlertSchemaMigration(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "legacy-alerts.db")

	// A database as written before this ticket: alert_events without
	// group_key, holding one legacy event.
	raw, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open raw sqlite: %v", err)
	}
	if _, err := raw.Exec(`CREATE TABLE alert_events (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		endpoint_id INTEGER,
		kind TEXT NOT NULL,
		message TEXT NOT NULL,
		sent_ok INTEGER NOT NULL,
		created_at TEXT NOT NULL
	)`); err != nil {
		t.Fatalf("create legacy alert_events: %v", err)
	}
	if _, err := raw.Exec(
		`INSERT INTO alert_events (endpoint_id, kind, message, sent_ok, created_at) VALUES (NULL, 'down', 'legacy outage', 1, ?)`,
		time.Now().UTC().Format(time.RFC3339),
	); err != nil {
		t.Fatalf("insert legacy event: %v", err)
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("close raw sqlite: %v", err)
	}

	// store.Open migrates in place; the legacy row survives and reads back
	// with a NULL group_key.
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("store.Open on legacy db: %v", err)
	}
	defer db.Close()
	seedTestUser(t, db)

	clock := scheduler.NewFakeClock(time.Now())
	ts := newAlertClockServer(t, db, clock)
	lark := newStubLarkServer(t)
	stubA := newStubHubServer()
	defer stubA.Close()
	stubB := newStubHubServer()
	defer stubB.Close()
	configureWebhook(t, ts, lark, true)

	events := listAlerts(t, ts, "")
	if len(events) != 1 || events[0]["message"].(string) != "legacy outage" {
		t.Fatalf("legacy event should survive the migration, got %v", events)
	}
	if events[0]["group_key"] != nil {
		t.Errorf("legacy event should read back with NULL group_key, got %v", events[0]["group_key"])
	}

	// Group alerts work on the migrated database.
	epA := createGroupEndpoint(t, ts, "Migrated Hub A", stubA.URL, "gpt-mig-a")
	epB := createGroupEndpoint(t, ts, "Migrated Hub B", stubB.URL, "gpt-mig-b")
	stubA.SetMode("error_503")
	stubB.SetMode("error_503")
	driveGroupEndpointDown(t, ts, epA)
	driveGroupEndpointDown(t, ts, epB)
	clock.Advance(alertWindowForTest)
	waitForLarkMessages(t, lark, 1)
	cards := lark.cards()
	if len(cards) != 1 || cards[0].Title != "厂商组告警:gpt · HubScope" {
		t.Fatalf("group alert should work on the migrated database, got %v", cards)
	}
	groupEvents := alertEventsOfKind(t, ts, "group_down")
	if len(groupEvents) != 1 || groupEvents[0]["group_key"] != "gpt" {
		t.Errorf("expected 1 group_down event with group_key gpt, got %v", groupEvents)
	}
}
