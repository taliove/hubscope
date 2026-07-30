package server_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/taliove/hubscope/internal/scheduler"
	"github.com/taliove/hubscope/internal/server"
	"github.com/taliove/hubscope/internal/store"
)

// alertWindowForTest mirrors the alerter's frozen 60s aggregation window
// (spec 0017, ADR 0014 — a constant, not a config surface). Tests advance
// the injected fake clock past it to flush the window with zero real
// waiting.
const alertWindowForTest = 60 * time.Second

// stubLarkServer is a fake Lark group-bot webhook endpoint. It records every
// message received — text messages as their raw text, interactive cards both
// parsed (cards) and flattened to plain text (so content assertions read one
// uniform stream via messages) — and can be switched to a failing mode that
// answers HTTP 500, or to a slow mode that delays the answer (used to prove
// senders never block their callers).
type stubLarkServer struct {
	*httptest.Server

	mu     sync.Mutex
	texts  []string
	seen   []larkCardSeen
	status int
	delay  time.Duration
}

// larkCardSeen is the parsed view of one interactive-card message received
// by the stub: the header color and title, the wide-screen flag, the div
// fields as a label→value map, the optional long-form detail block, and the
// note line.
type larkCardSeen struct {
	Template   string
	Title      string
	WideScreen bool
	Fields     map[string]string
	Detail     string
	Note       string
}

// stubCardPayload mirrors the legacy card JSON envelope the stub parses.
type stubCardPayload struct {
	Config struct {
		WideScreenMode bool `json:"wide_screen_mode"`
	} `json:"config"`
	Header struct {
		Template string `json:"template"`
		Title    struct {
			Content string `json:"content"`
		} `json:"title"`
	} `json:"header"`
	Elements []json.RawMessage `json:"elements"`
}

func newStubLarkServer(t *testing.T) *stubLarkServer {
	t.Helper()
	s := &stubLarkServer{status: http.StatusOK}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var payload struct {
			MsgType string `json:"msg_type"`
			Content struct {
				Text string `json:"text"`
			} `json:"content"`
			Card stubCardPayload `json:"card"`
		}
		_ = json.Unmarshal(body, &payload)
		s.mu.Lock()
		switch payload.MsgType {
		case "text":
			s.texts = append(s.texts, payload.Content.Text)
		case "interactive":
			card, flat := parseStubCard(payload.Card)
			s.seen = append(s.seen, card)
			s.texts = append(s.texts, flat)
		}
		status := s.status
		delay := s.delay
		s.mu.Unlock()
		if delay > 0 {
			time.Sleep(delay)
		}
		w.WriteHeader(status)
	}))
	t.Cleanup(s.Close)
	return s
}

// parseStubCard decodes the parsed view of one card plus a flattened plain
// text rendering (title + field contents + detail + note) so tests asserting
// on message content read cards and text messages uniformly.
func parseStubCard(payload stubCardPayload) (larkCardSeen, string) {
	card := larkCardSeen{
		Template:   payload.Header.Template,
		Title:      payload.Header.Title.Content,
		WideScreen: payload.Config.WideScreenMode,
		Fields:     map[string]string{},
	}
	flat := payload.Header.Title.Content
	for _, raw := range payload.Elements {
		var el struct {
			Tag    string `json:"tag"`
			Fields []struct {
				Text struct {
					Content string `json:"content"`
				} `json:"text"`
			} `json:"fields"`
			Text *struct {
				Content string `json:"content"`
			} `json:"text"`
			Elements []struct {
				Content string `json:"content"`
			} `json:"elements"`
		}
		if err := json.Unmarshal(raw, &el); err != nil {
			continue
		}
		switch el.Tag {
		case "div":
			for _, f := range el.Fields {
				if label, value, ok := parseStubField(f.Text.Content); ok {
					card.Fields[label] = value
				}
				flat += "\n" + f.Text.Content
			}
			if el.Text != nil {
				card.Detail = el.Text.Content
				flat += "\n" + el.Text.Content
			}
		case "note":
			for _, n := range el.Elements {
				card.Note += n.Content
				flat += "\n" + n.Content
			}
		}
	}
	return card, flat
}

// parseStubField splits one field's lark_md content ("**label**\nvalue")
// into its label and value.
func parseStubField(content string) (label, value string, ok bool) {
	if !strings.HasPrefix(content, "**") {
		return "", "", false
	}
	rest := content[2:]
	idx := strings.Index(rest, "**\n")
	if idx < 0 {
		return "", "", false
	}
	return rest[:idx], rest[idx+len("**\n"):], true
}

// messages returns a copy of the recorded message stream: raw text for text
// messages, the flattened rendering for interactive cards. A failed delivery
// (stub answering 500) still appears here — it records delivery attempts.
func (s *stubLarkServer) messages() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, len(s.texts))
	copy(out, s.texts)
	return out
}

// cards returns a copy of the parsed interactive cards received so far.
func (s *stubLarkServer) cards() []larkCardSeen {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]larkCardSeen, len(s.seen))
	copy(out, s.seen)
	return out
}

// setStatus changes the HTTP status the stub answers with.
func (s *stubLarkServer) setStatus(status int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status = status
}

// setDelay makes the stub hold each request for the given duration before
// answering, simulating a slow webhook.
func (s *stubLarkServer) setDelay(delay time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.delay = delay
}

// newAlertClockServer starts the full API server with a fake clock driving
// the alert aggregation window (server.WithAlertClock, spec 0017): the 60s
// window only flushes when the test advances the clock, so "nothing sent
// before the flush" assertions are deterministic and "after the flush" ones
// synchronize via waitForLarkMessages/waitForAlertEvents.
func newAlertClockServer(t *testing.T, db *store.DB, clock *scheduler.FakeClock) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(server.New(db,
		server.WithRateLimits(server.RateLimits{}),
		server.WithSessionSecret(testSessionSecret),
		server.WithSyncEval(),
		server.WithSyncDiscovery(),
		server.WithAlertClock(clock),
	))
	t.Cleanup(ts.Close)
	return ts
}

// createProbedEndpoint registers a hub+model against the given base URL and
// returns the ID of its anthropic endpoint.
func createProbedEndpoint(t *testing.T, ts *httptest.Server, hubName, baseURL, modelID string) int64 {
	t.Helper()
	hubID := createHubForAlerts(t, ts, hubName, baseURL)
	return createAnthropicEndpoint(t, ts, hubID, modelID)
}

// createHubForAlerts registers one hub against the given base URL.
func createHubForAlerts(t *testing.T, ts *httptest.Server, hubName, baseURL string) int64 {
	t.Helper()

	hubResp := doPost(t, ts.URL+"/api/hubs", map[string]interface{}{
		"name":     hubName,
		"base_url": baseURL,
		"token":    "fake-token-0000",
	})
	var env envelope
	if err := json.NewDecoder(hubResp.Body).Decode(&env); err != nil {
		t.Fatalf("decode hub: %v", err)
	}
	hubResp.Body.Close()
	var hub map[string]interface{}
	if err := json.Unmarshal(env.Data, &hub); err != nil {
		t.Fatalf("unmarshal hub: %v", err)
	}
	return int64(hub["id"].(float64))
}

// createAnthropicEndpoint registers one model under an existing hub and
// returns the ID of its anthropic endpoint.
func createAnthropicEndpoint(t *testing.T, ts *httptest.Server, hubID int64, modelID string) int64 {
	t.Helper()

	modelResp := doPost(t, ts.URL+"/api/models", map[string]interface{}{
		"hub_id":   hubID,
		"model_id": modelID,
	})
	if modelResp.StatusCode != http.StatusCreated {
		t.Fatalf("create model: expected 201, got %d", modelResp.StatusCode)
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
	endpoints := model["endpoints"].([]interface{})
	for _, raw := range endpoints {
		ep := raw.(map[string]interface{})
		if ep["protocol"].(string) == "anthropic" {
			return int64(ep["id"].(float64))
		}
	}
	t.Fatal("anthropic endpoint not found")
	return 0
}

// runProbeRound triggers one manual probe round and requires success.
func runProbeRound(t *testing.T, ts *httptest.Server, endpointID int64) {
	t.Helper()
	resp := doPost(t, ts.URL+"/api/endpoints/"+itoa(endpointID)+"/probe", nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("probe round: expected 200, got %d", resp.StatusCode)
	}
}

// listAlerts fetches GET /api/alerts with the given raw query suffix.
func listAlerts(t *testing.T, ts *httptest.Server, query string) []map[string]interface{} {
	t.Helper()
	resp := doGet(t, ts.URL+"/api/alerts"+query)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list alerts: expected 200, got %d", resp.StatusCode)
	}
	var env envelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode alerts: %v", err)
	}
	var events []map[string]interface{}
	if err := json.Unmarshal(env.Data, &events); err != nil {
		t.Fatalf("unmarshal alerts: %v", err)
	}
	return events
}

// alertEventsOfKind (defined in eval_score_drop_test.go) filters alert
// events by kind; this file uses it for down/recovered/batch kinds.

// waitForAlertEvents polls GET /api/alerts until it holds at least n events.
// The window flush writes events back (sent_ok + the batch event) after the
// webhook delivery, so event-count assertions synchronize on the API rather
// than on message arrival.
func waitForAlertEvents(t *testing.T, ts *httptest.Server, n int) []map[string]interface{} {
	t.Helper()
	var events []map[string]interface{}
	waitFor(t, fmt.Sprintf("%d alert events", n), func() bool {
		events = listAlerts(t, ts, "")
		return len(events) >= n
	})
	return events
}

// itoa renders an endpoint ID for URL construction.
func itoa(n int64) string {
	return strconv.FormatInt(n, 10)
}

// TestLarkAlertingLifecycle covers the alert lifecycle under the aggregation
// window (spec 0017, ADR 0014): three consecutive failures record the down
// event at decision time (sent_ok=false, "delivery unconfirmed") but send
// nothing until the fake clock flushes the 60s window; the outage then stays
// silent, and a recovery rides the next window flush. The sent text is one
// aggregated card per kind, and a batch event records what actually went
// out.
func TestLarkAlertingLifecycle(t *testing.T) {
	db := openTempDB(t)
	clock := scheduler.NewFakeClock(time.Now())
	ts := newAlertClockServer(t, db, clock)
	lark := newStubLarkServer(t)
	stubHub := newStubHubServer()
	defer stubHub.Close()

	// Point alerting at the stub webhook.
	configureWebhook(t, ts, lark, true)

	endpointID := createProbedEndpoint(t, ts, "Alert Hub", stubHub.URL, "alert-model")
	stubHub.SetMode("error_503")

	// Round 1: two failed probes (non-streaming + streaming), below the
	// threshold of 3 consecutive failures — no event, no message.
	runProbeRound(t, ts, endpointID)
	if got := len(lark.messages()); got != 0 {
		t.Fatalf("after 2 failures: expected no messages, got %d", got)
	}
	if got := listAlerts(t, ts, ""); len(got) != 0 {
		t.Fatalf("after 2 failures: expected no events, got %d", len(got))
	}

	// Round 2: failures 3 and 4 cross the threshold. The transition is
	// decided now: the event lands immediately with sent_ok=false
	// ("delivery unconfirmed"), but nothing is sent before the window
	// flushes.
	runProbeRound(t, ts, endpointID)
	events := listAlerts(t, ts, "")
	if len(events) != 1 || events[0]["kind"].(string) != "down" {
		t.Fatalf("transition should record the down event at decision time, got %v", events)
	}
	if events[0]["sent_ok"].(bool) != false {
		t.Error("event at decision time: expected sent_ok false (delivery unconfirmed)")
	}
	if int64(events[0]["endpoint_id"].(float64)) != endpointID {
		t.Errorf("expected endpoint_id %d, got %v", endpointID, events[0]["endpoint_id"])
	}
	if got := len(lark.messages()); got != 0 {
		t.Fatalf("before window flush: expected no messages, got %d", got)
	}

	// Flush the 60s window: exactly one aggregated down card.
	clock.Advance(alertWindowForTest)
	msgs := waitForLarkMessages(t, lark, 1)
	for _, want := range []string{"alert-model", "anthropic", "HTTP 503"} {
		if !strings.Contains(msgs[0], want) {
			t.Errorf("down alert should contain %q, got: %s", want, msgs[0])
		}
	}

	// The webhook receives one interactive card: red header with the alert
	// title, the per-hub section carrying model/protocol/error in the
	// detail block, wide-screen on, and the service signature in the note.
	waitForAlertEvents(t, ts, 2) // down + batch: flush fully settled
	if got := len(lark.messages()); got != 1 {
		t.Fatalf("one down transition: expected exactly 1 message, got %d", got)
	}
	cards := lark.cards()
	if len(cards) != 1 {
		t.Fatalf("expected 1 card, got %d", len(cards))
	}
	down := cards[0]
	if down.Template != "red" {
		t.Errorf("down card template: expected red, got %q", down.Template)
	}
	if down.Title != "端点告警 · HubScope" {
		t.Errorf("down card title: got %q", down.Title)
	}
	if !down.WideScreen {
		t.Error("down card should set wide_screen_mode")
	}
	for _, want := range []string{"Alert Hub", "alert-model", "anthropic", "HTTP 503"} {
		if !strings.Contains(down.Detail, want) {
			t.Errorf("down card detail should contain %q, got: %s", want, down.Detail)
		}
	}
	if !strings.Contains(down.Note, "HubScope 服务监控") {
		t.Errorf("down card note should carry the service signature, got %q", down.Note)
	}

	// After the flush the down event's sent_ok is written back, and one
	// hub-less batch event records the actual aggregated text (spec 0017
	// story 31: the history shows what readers saw).
	downEvents := alertEventsOfKind(t, ts, "down")
	if len(downEvents) != 1 {
		t.Fatalf("expected 1 down event, got %d", len(downEvents))
	}
	if downEvents[0]["sent_ok"].(bool) != true {
		t.Error("down event sent_ok should be written back to true after the flush")
	}
	if !strings.Contains(downEvents[0]["message"].(string), "alert-model") {
		t.Errorf("down event message should name the model, got: %v", downEvents[0]["message"])
	}
	batchEvents := alertEventsOfKind(t, ts, "batch")
	if len(batchEvents) != 1 {
		t.Fatalf("expected 1 batch event, got %d", len(batchEvents))
	}
	if batchEvents[0]["sent_ok"].(bool) != true {
		t.Error("batch event should record the real delivery result (true)")
	}
	if batchEvents[0]["endpoint_id"] != nil {
		t.Errorf("batch event should be hub-less, got endpoint_id %v", batchEvents[0]["endpoint_id"])
	}
	batchMessage := batchEvents[0]["message"].(string)
	for _, want := range []string{"alert-model", "anthropic"} {
		if !strings.Contains(batchMessage, want) {
			t.Errorf("batch event message should contain %q, got: %s", want, batchMessage)
		}
	}
	// The persisted message stays plain text (the alert history table
	// renders it unchanged) — never the card JSON.
	if !strings.HasPrefix(batchMessage, "【HubScope】") || strings.Contains(batchMessage, "msg_type") {
		t.Errorf("batch event message must stay plain text, got: %v", batchMessage)
	}

	// Rounds 3 and 4: the outage continues — silence, and another flush
	// cycle sends nothing (no new transitions were buffered).
	runProbeRound(t, ts, endpointID)
	runProbeRound(t, ts, endpointID)
	clock.Advance(alertWindowForTest)
	if got := len(lark.messages()); got != 1 {
		t.Fatalf("ongoing outage: expected still 1 message, got %d", got)
	}
	if got := listAlerts(t, ts, ""); len(got) != 2 {
		t.Fatalf("ongoing outage: expected still 2 events, got %d", len(got))
	}

	// Recovery: the hub answers successfully again. The recovered
	// transition buffers silently and rides the next window flush.
	stubHub.SetMode("success")
	runProbeRound(t, ts, endpointID)
	if got := len(lark.messages()); got != 1 {
		t.Fatalf("before recovery flush: expected still 1 message, got %d", got)
	}
	clock.Advance(alertWindowForTest)
	msgs = waitForLarkMessages(t, lark, 2)
	if !strings.Contains(msgs[1], "alert-model") {
		t.Errorf("recovered alert should name the model, got: %s", msgs[1])
	}
	waitForAlertEvents(t, ts, 4) // down + batch + recovered + batch
	cards = lark.cards()
	if len(cards) != 2 {
		t.Fatalf("after recovery: expected 2 cards, got %d", len(cards))
	}
	if cards[1].Template != "green" {
		t.Errorf("recovered card template: expected green, got %q", cards[1].Template)
	}
	if cards[1].Title != "端点恢复 · HubScope" {
		t.Errorf("recovered card title: got %q", cards[1].Title)
	}
	if !strings.Contains(cards[1].Detail, "alert-model") {
		t.Errorf("recovered card detail should name the model, got: %s", cards[1].Detail)
	}
	if got := alertEventsOfKind(t, ts, "recovered"); len(got) != 1 || got[0]["sent_ok"].(bool) != true {
		t.Errorf("expected 1 recovered event with sent_ok true, got %v", got)
	}
	// Newest-first ordering: the down event (oldest) trails the list.
	if all := listAlerts(t, ts, ""); all[len(all)-1]["kind"].(string) != "down" {
		t.Errorf("expected oldest event to be the down event (newest-first), got %v", all[len(all)-1]["kind"])
	}
	if got := alertEventsOfKind(t, ts, "batch"); len(got) != 2 {
		t.Errorf("expected 2 batch events, got %d", len(got))
	}

	// The limit parameter is honored.
	if got := listAlerts(t, ts, "?limit=1"); len(got) != 1 {
		t.Errorf("limit=1: expected 1 event, got %d", len(got))
	}
}

// TestAlertWindowAggregatesDownAcrossHubs drives three endpoints on two hubs
// down inside one window: the flush sends exactly one red card grouping the
// endpoints by hub, and the hub holding two of them is flagged as a
// suspected hub-side fault.
func TestAlertWindowAggregatesDownAcrossHubs(t *testing.T) {
	db := openTempDB(t)
	clock := scheduler.NewFakeClock(time.Now())
	ts := newAlertClockServer(t, db, clock)
	lark := newStubLarkServer(t)
	stubA := newStubHubServer()
	defer stubA.Close()
	stubB := newStubHubServer()
	defer stubB.Close()

	configureWebhook(t, ts, lark, true)

	hubA := createHubForAlerts(t, ts, "Alpha Hub", stubA.URL)
	epA1 := createAnthropicEndpoint(t, ts, hubA, "alpha-m1")
	epA2 := createAnthropicEndpoint(t, ts, hubA, "alpha-m2")
	hubB := createHubForAlerts(t, ts, "Beta Hub", stubB.URL)
	epB1 := createAnthropicEndpoint(t, ts, hubB, "beta-m1")
	stubA.SetMode("error_503")
	stubB.SetMode("error_503")

	// All three endpoints cross the threshold inside one window.
	for _, ep := range []int64{epA1, epA2, epB1} {
		runProbeRound(t, ts, ep)
		runProbeRound(t, ts, ep)
	}
	if got := len(lark.messages()); got != 0 {
		t.Fatalf("before window flush: expected no messages, got %d", got)
	}

	clock.Advance(alertWindowForTest)
	waitForLarkMessages(t, lark, 1)
	waitForAlertEvents(t, ts, 4) // 3 down + 1 batch: flush settled
	if got := len(lark.messages()); got != 1 {
		t.Fatalf("three down transitions in one window: expected exactly 1 aggregated card, got %d", got)
	}

	cards := lark.cards()
	if len(cards) != 1 {
		t.Fatalf("expected 1 card, got %d", len(cards))
	}
	down := cards[0]
	if down.Template != "red" {
		t.Errorf("aggregated down card template: expected red, got %q", down.Template)
	}
	if down.Title != "端点告警 · HubScope" {
		t.Errorf("aggregated down card title: got %q", down.Title)
	}
	// Every model appears, grouped under its hub; the two-endpoint hub
	// carries the suspected-hub-fault flag, the single-endpoint hub does
	// not.
	for _, want := range []string{"alpha-m1", "alpha-m2", "beta-m1", "anthropic", "HTTP 503"} {
		if !strings.Contains(down.Detail, want) {
			t.Errorf("aggregated detail should contain %q, got: %s", want, down.Detail)
		}
	}
	if !strings.Contains(down.Detail, "Alpha Hub(疑似 Hub 侧故障)") {
		t.Errorf("two-endpoint hub should be flagged as suspected hub-side fault, got: %s", down.Detail)
	}
	if !strings.Contains(down.Detail, "Beta Hub") {
		t.Errorf("single-endpoint hub should still get its own section, got: %s", down.Detail)
	}
	if strings.Contains(down.Detail, "Beta Hub(疑似 Hub 侧故障)") {
		t.Errorf("single-endpoint hub must not be flagged as suspected hub-side fault, got: %s", down.Detail)
	}

	// All three down events are written back sent_ok=true; the batch event
	// carries the aggregated text naming every model and the hub flag.
	if got := alertEventsOfKind(t, ts, "down"); len(got) != 3 {
		t.Fatalf("expected 3 down events, got %d", len(got))
	} else {
		for _, e := range got {
			if e["sent_ok"].(bool) != true {
				t.Errorf("down event for endpoint %v: sent_ok should be true after the flush", e["endpoint_id"])
			}
		}
	}
	batch := alertEventsOfKind(t, ts, "batch")
	if len(batch) != 1 {
		t.Fatalf("expected 1 batch event, got %d", len(batch))
	}
	batchMessage := batch[0]["message"].(string)
	for _, want := range []string{"alpha-m1", "alpha-m2", "beta-m1", "疑似 Hub 侧故障"} {
		if !strings.Contains(batchMessage, want) {
			t.Errorf("batch event message should contain %q, got: %s", want, batchMessage)
		}
	}
}

// TestAlertWindowAggregatesRecovery merges same-window recoveries into one
// green card, grouped by hub like the down card.
func TestAlertWindowAggregatesRecovery(t *testing.T) {
	db := openTempDB(t)
	clock := scheduler.NewFakeClock(time.Now())
	ts := newAlertClockServer(t, db, clock)
	lark := newStubLarkServer(t)
	stubHub := newStubHubServer()
	defer stubHub.Close()

	configureWebhook(t, ts, lark, true)

	hubID := createHubForAlerts(t, ts, "Heal Hub", stubHub.URL)
	ep1 := createAnthropicEndpoint(t, ts, hubID, "heal-m1")
	ep2 := createAnthropicEndpoint(t, ts, hubID, "heal-m2")
	stubHub.SetMode("error_503")

	// Both endpoints go down and flush as one card.
	for _, ep := range []int64{ep1, ep2} {
		runProbeRound(t, ts, ep)
		runProbeRound(t, ts, ep)
	}
	clock.Advance(alertWindowForTest)
	waitForLarkMessages(t, lark, 1)
	waitForAlertEvents(t, ts, 3) // 2 down + 1 batch

	// Both recover inside the next window: still nothing until the flush.
	stubHub.SetMode("success")
	runProbeRound(t, ts, ep1)
	runProbeRound(t, ts, ep2)
	if got := len(lark.messages()); got != 1 {
		t.Fatalf("before recovery flush: expected still 1 message, got %d", got)
	}
	clock.Advance(alertWindowForTest)
	waitForLarkMessages(t, lark, 2)
	waitForAlertEvents(t, ts, 6) // +2 recovered +1 batch
	if got := len(lark.messages()); got != 2 {
		t.Fatalf("two recoveries in one window: expected exactly 2 messages total, got %d", got)
	}

	cards := lark.cards()
	if len(cards) != 2 {
		t.Fatalf("expected 2 cards, got %d", len(cards))
	}
	recovered := cards[1]
	if recovered.Template != "green" {
		t.Errorf("aggregated recovery card template: expected green, got %q", recovered.Template)
	}
	if recovered.Title != "端点恢复 · HubScope" {
		t.Errorf("aggregated recovery card title: got %q", recovered.Title)
	}
	for _, want := range []string{"Heal Hub", "heal-m1", "heal-m2"} {
		if !strings.Contains(recovered.Detail, want) {
			t.Errorf("aggregated recovery detail should contain %q, got: %s", want, recovered.Detail)
		}
	}
	if got := alertEventsOfKind(t, ts, "recovered"); len(got) != 2 {
		t.Errorf("expected 2 recovered events, got %d", len(got))
	}
	if got := alertEventsOfKind(t, ts, "batch"); len(got) != 2 {
		t.Errorf("expected 2 batch events, got %d", len(got))
	}
}

// TestAlertWindowSplitsDownAndRecovery proves bad news and good news never
// share a card: a down and a recovery buffered in the same window flush as
// two separate cards.
func TestAlertWindowSplitsDownAndRecovery(t *testing.T) {
	db := openTempDB(t)
	clock := scheduler.NewFakeClock(time.Now())
	ts := newAlertClockServer(t, db, clock)
	lark := newStubLarkServer(t)
	stubA := newStubHubServer()
	defer stubA.Close()
	stubB := newStubHubServer()
	defer stubB.Close()

	configureWebhook(t, ts, lark, true)

	hubA := createHubForAlerts(t, ts, "Split Hub A", stubA.URL)
	epA := createAnthropicEndpoint(t, ts, hubA, "split-model-a")
	hubB := createHubForAlerts(t, ts, "Split Hub B", stubB.URL)
	epB := createAnthropicEndpoint(t, ts, hubB, "split-model-b")

	// epB goes down first and its alert flushes, so it can recover later.
	stubB.SetMode("error_503")
	runProbeRound(t, ts, epB)
	runProbeRound(t, ts, epB)
	clock.Advance(alertWindowForTest)
	waitForLarkMessages(t, lark, 1)
	waitForAlertEvents(t, ts, 2) // 1 down + 1 batch

	// Same window: epB recovers while epA crosses the down threshold.
	stubA.SetMode("error_503")
	stubB.SetMode("success")
	runProbeRound(t, ts, epA) // failures 1-2
	runProbeRound(t, ts, epB) // recovery transition: arms the window
	runProbeRound(t, ts, epA) // failures 3-4: down transition joins the window
	if got := len(lark.messages()); got != 1 {
		t.Fatalf("before split flush: expected still 1 message, got %d", got)
	}
	clock.Advance(alertWindowForTest)
	waitForLarkMessages(t, lark, 3)
	waitForAlertEvents(t, ts, 6) // +1 recovered +1 batch +1 down +1 batch
	if got := len(lark.messages()); got != 3 {
		t.Fatalf("down + recovery in one window: expected 3 messages total, got %d", got)
	}

	cards := lark.cards()
	if len(cards) != 3 {
		t.Fatalf("expected 3 cards, got %d", len(cards))
	}
	downCard, recoveredCard := cards[1], cards[2]
	if downCard.Template != "red" || !strings.Contains(downCard.Detail, "split-model-a") {
		t.Errorf("second card should be the red down card for split-model-a, got template %q detail %q",
			downCard.Template, downCard.Detail)
	}
	if recoveredCard.Template != "green" || !strings.Contains(recoveredCard.Detail, "split-model-b") {
		t.Errorf("third card should be the green recovery card for split-model-b, got template %q detail %q",
			recoveredCard.Template, recoveredCard.Detail)
	}
	if got := alertEventsOfKind(t, ts, "down"); len(got) != 2 {
		t.Errorf("expected 2 down events, got %d", len(got))
	}
	if got := alertEventsOfKind(t, ts, "recovered"); len(got) != 1 {
		t.Errorf("expected 1 recovered event, got %d", len(got))
	}
	if got := alertEventsOfKind(t, ts, "batch"); len(got) != 3 {
		t.Errorf("expected 3 batch events, got %d", len(got))
	}
}

// TestAlertSkippedWithoutWebhook verifies that an unconfigured webhook (or a
// disabled alert switch) produces no events and no errors while probes fail.
func TestAlertSkippedWithoutWebhook(t *testing.T) {
	db := openTempDB(t)
	ts := newTestAPIServer(t, db)
	lark := newStubLarkServer(t)
	stubHub := newStubHubServer()
	defer stubHub.Close()

	endpointID := createProbedEndpoint(t, ts, "Silent Hub", stubHub.URL, "silent-model")
	stubHub.SetMode("error_503")

	// Webhook never configured: failing rounds produce no events, no errors.
	runProbeRound(t, ts, endpointID)
	runProbeRound(t, ts, endpointID)
	runProbeRound(t, ts, endpointID)
	if got := listAlerts(t, ts, ""); len(got) != 0 {
		t.Fatalf("no webhook configured: expected 0 events, got %d", len(got))
	}

	// Webhook configured but alerts disabled: still nothing sent, no events.
	putResp := doPut(t, ts.URL+"/api/settings", map[string]interface{}{
		"lark_webhook_url": lark.URL,
		"alert_enabled":    false,
	})
	putResp.Body.Close()
	if putResp.StatusCode != http.StatusOK {
		t.Fatalf("put settings: expected 200, got %d", putResp.StatusCode)
	}
	runProbeRound(t, ts, endpointID)
	if got := len(lark.messages()); got != 0 {
		t.Fatalf("alerts disabled: expected no messages, got %d", got)
	}
	if got := listAlerts(t, ts, ""); len(got) != 0 {
		t.Fatalf("alerts disabled: expected 0 events, got %d", len(got))
	}
}

// TestAlertSendFailureRecorded verifies that a webhook answering 500 leaves
// the buffered events with sent_ok=false after the flush, records the failed
// batch attempt, and never retries on subsequent rounds (spec 0017: a failed
// aggregated send does not re-arm anything).
func TestAlertSendFailureRecorded(t *testing.T) {
	db := openTempDB(t)
	clock := scheduler.NewFakeClock(time.Now())
	ts := newAlertClockServer(t, db, clock)
	lark := newStubLarkServer(t)
	lark.setStatus(http.StatusInternalServerError)
	stubHub := newStubHubServer()
	defer stubHub.Close()

	configureWebhook(t, ts, lark, true)

	endpointID := createProbedEndpoint(t, ts, "Fail Hub", stubHub.URL, "fail-model")
	stubHub.SetMode("error_503")

	runProbeRound(t, ts, endpointID)
	runProbeRound(t, ts, endpointID)
	if got := len(lark.messages()); got != 0 {
		t.Fatalf("before window flush: expected no delivery attempts, got %d", got)
	}

	// The flush attempts the aggregated send once; the stub records the
	// attempt even though it answers 500.
	clock.Advance(alertWindowForTest)
	waitForLarkMessages(t, lark, 1)
	waitForAlertEvents(t, ts, 2) // down + batch: flush settled
	downEvents := alertEventsOfKind(t, ts, "down")
	if len(downEvents) != 1 {
		t.Fatalf("expected 1 down event, got %d", len(downEvents))
	}
	if downEvents[0]["sent_ok"].(bool) != false {
		t.Error("down event should stay sent_ok=false when the webhook fails")
	}
	batchEvents := alertEventsOfKind(t, ts, "batch")
	if len(batchEvents) != 1 {
		t.Fatalf("expected 1 batch event recording the failed attempt, got %d", len(batchEvents))
	}
	if batchEvents[0]["sent_ok"].(bool) != false {
		t.Error("batch event should record the real delivery result (false)")
	}

	// A failed send does not re-arm the alert: further failures stay quiet,
	// and the next window cycle attempts nothing.
	runProbeRound(t, ts, endpointID)
	clock.Advance(alertWindowForTest)
	if got := len(lark.messages()); got != 1 {
		t.Fatalf("failed send must not be retried: expected still 1 delivery attempt, got %d", got)
	}
	if got := listAlerts(t, ts, ""); len(got) != 2 {
		t.Fatalf("expected still 2 events after another failing round, got %d", len(got))
	}
}

// TestAlertRestartDoesNotRepeat simulates a service restart: a fresh server
// (fresh in-memory alert state) over the same database must not re-alert an
// endpoint whose down event was already recorded. Each "process" gets its
// own fake clock — the dead process's clock is never advanced again, which
// is what dying does to its pending window.
func TestAlertRestartDoesNotRepeat(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "restart.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	seedTestUser(t, db)

	lark := newStubLarkServer(t)
	stubHub := newStubHubServer()
	defer stubHub.Close()

	// First process: configure the webhook, drive the endpoint down, and
	// flush the window.
	clock1 := scheduler.NewFakeClock(time.Now())
	ts1 := httptest.NewServer(server.New(db,
		server.WithRateLimits(server.RateLimits{}),
		server.WithSyncDiscovery(),
		server.WithAlertClock(clock1),
	))
	configureWebhook(t, ts1, lark, true)
	endpointID := createProbedEndpoint(t, ts1, "Restart Hub", stubHub.URL, "restart-model")
	stubHub.SetMode("error_503")
	runProbeRound(t, ts1, endpointID)
	runProbeRound(t, ts1, endpointID)
	clock1.Advance(alertWindowForTest)
	waitForLarkMessages(t, lark, 1)
	waitForAlertEvents(t, ts1, 2) // down + batch
	ts1.Close()

	// "Restart": a brand-new server (fresh alert evaluator, fresh clock)
	// over the same database. Another failing round must not re-send the
	// down alert — the persisted event suppresses it.
	clock2 := scheduler.NewFakeClock(time.Now())
	ts2 := httptest.NewServer(server.New(db,
		server.WithRateLimits(server.RateLimits{}),
		server.WithSyncDiscovery(),
		server.WithAlertClock(clock2),
	))
	defer ts2.Close()
	runProbeRound(t, ts2, endpointID)
	clock2.Advance(alertWindowForTest)
	if got := len(lark.messages()); got != 1 {
		t.Fatalf("after restart: expected still 1 message, got %d", got)
	}
	if got := alertEventsOfKind(t, ts2, "down"); len(got) != 1 {
		t.Fatalf("after restart: expected still 1 down event, got %d", len(got))
	}

	// Recovery after the restart still fires exactly once, via the window.
	stubHub.SetMode("success")
	runProbeRound(t, ts2, endpointID)
	if got := len(lark.messages()); got != 1 {
		t.Fatalf("before recovery flush: expected still 1 message, got %d", got)
	}
	clock2.Advance(alertWindowForTest)
	waitForLarkMessages(t, lark, 2)
}

// TestAlertWindowRestartDropsBuffer kills the "process" while a down
// transition sits unflushed in the window: the buffer is lost, but the event
// was recorded at decision time, so the restarted process neither re-alerts
// nor fabricates a delivery — the event honestly stays sent_ok=false
// ("delivery unconfirmed", ADR 0014).
func TestAlertWindowRestartDropsBuffer(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "restart-buffer.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	seedTestUser(t, db)

	lark := newStubLarkServer(t)
	stubHub := newStubHubServer()
	defer stubHub.Close()

	// First process: drive the endpoint down but never flush the window.
	clock1 := scheduler.NewFakeClock(time.Now())
	ts1 := httptest.NewServer(server.New(db,
		server.WithRateLimits(server.RateLimits{}),
		server.WithSyncDiscovery(),
		server.WithAlertClock(clock1),
	))
	configureWebhook(t, ts1, lark, true)
	endpointID := createProbedEndpoint(t, ts1, "Buffer Hub", stubHub.URL, "buffer-model")
	stubHub.SetMode("error_503")
	runProbeRound(t, ts1, endpointID)
	runProbeRound(t, ts1, endpointID)
	events := listAlerts(t, ts1, "")
	if len(events) != 1 || events[0]["kind"].(string) != "down" {
		t.Fatalf("transition should record the down event at decision time, got %v", events)
	}
	if got := len(lark.messages()); got != 0 {
		t.Fatalf("unflushed window: expected no messages, got %d", got)
	}
	ts1.Close()

	// "Restart" with a fresh clock: the still-failing endpoint must not
	// re-alert (its event is already persisted), and advancing the new
	// process's window sends nothing — the dead process's buffer is gone.
	clock2 := scheduler.NewFakeClock(time.Now())
	ts2 := httptest.NewServer(server.New(db,
		server.WithRateLimits(server.RateLimits{}),
		server.WithSyncDiscovery(),
		server.WithAlertClock(clock2),
	))
	defer ts2.Close()
	runProbeRound(t, ts2, endpointID)
	clock2.Advance(alertWindowForTest)
	if got := len(lark.messages()); got != 0 {
		t.Fatalf("restart with a dropped buffer must not re-alert, got %d messages", got)
	}
	events = listAlerts(t, ts2, "")
	if len(events) != 1 {
		t.Fatalf("expected exactly the original down event, got %v", events)
	}
	if events[0]["sent_ok"].(bool) != false {
		t.Error("unflushed event should honestly stay sent_ok=false (delivery unconfirmed)")
	}
}
