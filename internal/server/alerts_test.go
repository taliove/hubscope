package server_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/taliove/hubscope/internal/server"
	"github.com/taliove/hubscope/internal/store"
)

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
// messages, the flattened rendering for interactive cards.
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

// createProbedEndpoint registers a hub+model against the given base URL and
// returns the ID of its anthropic endpoint.
func createProbedEndpoint(t *testing.T, ts *httptest.Server, hubName, baseURL, modelID string) int64 {
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
	hubID := int64(hub["id"].(float64))

	modelResp := doPost(t, ts.URL+"/api/models", map[string]interface{}{
		"hub_id":   hubID,
		"model_id": modelID,
	})
	if modelResp.StatusCode != http.StatusCreated {
		t.Fatalf("create model: expected 201, got %d", modelResp.StatusCode)
	}
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

// itoa renders an endpoint ID for URL construction.
func itoa(n int64) string {
	return strconv.FormatInt(n, 10)
}

// TestLarkAlertingLifecycle covers the ticket 06 acceptance flow: three
// consecutive failures fire exactly one down alert, further failures stay
// silent, and a recovery fires one recovered notification.
func TestLarkAlertingLifecycle(t *testing.T) {
	db := openTempDB(t)
	ts := newTestAPIServer(t, db)
	lark := newStubLarkServer(t)
	stubHub := newStubHubServer()
	defer stubHub.Close()

	// Point alerting at the stub webhook.
	putResp := doPut(t, ts.URL+"/api/settings", map[string]interface{}{
		"lark_webhook_url": lark.URL,
		"alert_enabled":    true,
	})
	putResp.Body.Close()
	if putResp.StatusCode != http.StatusOK {
		t.Fatalf("put settings: expected 200, got %d", putResp.StatusCode)
	}

	endpointID := createProbedEndpoint(t, ts, "Alert Hub", stubHub.URL, "alert-model")
	stubHub.SetMode("error_503")

	// Round 1: two failed probes (non-streaming + streaming), below the
	// threshold of 3 consecutive failures, so no alert yet.
	runProbeRound(t, ts, endpointID)
	if got := len(lark.messages()); got != 0 {
		t.Fatalf("after 2 failures: expected no messages, got %d", got)
	}

	// Round 2: failures 3 and 4 cross the threshold — exactly one down alert.
	runProbeRound(t, ts, endpointID)
	msgs := lark.messages()
	if len(msgs) != 1 {
		t.Fatalf("after crossing threshold: expected 1 message, got %d", len(msgs))
	}
	for _, want := range []string{"alert-model", "anthropic", "HTTP 503"} {
		if !strings.Contains(msgs[0], want) {
			t.Errorf("down alert should contain %q, got: %s", want, msgs[0])
		}
	}

	// The webhook receives an interactive card (ticket 101): red header with
	// the alert title, structured fields carrying model/protocol/error,
	// wide-screen on, and the service signature in the note.
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
	if down.Fields["模型"] != "alert-model" {
		t.Errorf("down card 模型 field: got %q", down.Fields["模型"])
	}
	if down.Fields["协议"] != "anthropic" {
		t.Errorf("down card 协议 field: got %q", down.Fields["协议"])
	}
	if !strings.Contains(down.Fields["最近错误"], "HTTP 503") {
		t.Errorf("down card 最近错误 field should carry the error summary, got %q", down.Fields["最近错误"])
	}
	if !strings.Contains(down.Note, "HubScope 服务监控") {
		t.Errorf("down card note should carry the service signature, got %q", down.Note)
	}

	// Rounds 3 and 4: the outage continues, no repeat alert.
	runProbeRound(t, ts, endpointID)
	runProbeRound(t, ts, endpointID)
	if got := len(lark.messages()); got != 1 {
		t.Fatalf("ongoing outage: expected still 1 message, got %d", got)
	}

	// The down alert event is persisted and visible via the API.
	events := listAlerts(t, ts, "")
	if len(events) != 1 {
		t.Fatalf("expected 1 alert event, got %d", len(events))
	}
	if events[0]["kind"].(string) != "down" {
		t.Errorf("expected kind down, got %v", events[0]["kind"])
	}
	if events[0]["sent_ok"].(bool) != true {
		t.Error("expected sent_ok true")
	}
	if int64(events[0]["endpoint_id"].(float64)) != endpointID {
		t.Errorf("expected endpoint_id %d, got %v", endpointID, events[0]["endpoint_id"])
	}
	if !strings.Contains(events[0]["message"].(string), "alert-model") {
		t.Errorf("event message should name the model, got: %v", events[0]["message"])
	}
	// The persisted message stays plain text (ticket 101 regression: the
	// alert history table renders it unchanged) — never the card JSON.
	if msg := events[0]["message"].(string); !strings.HasPrefix(msg, "【HubScope】端点告警") ||
		strings.Contains(msg, "msg_type") {
		t.Errorf("event message must stay plain text, got: %v", msg)
	}

	// Recovery: the hub answers successfully again — one recovered notice.
	stubHub.SetMode("success")
	runProbeRound(t, ts, endpointID)
	msgs = lark.messages()
	if len(msgs) != 2 {
		t.Fatalf("after recovery: expected 2 messages, got %d", len(msgs))
	}
	if !strings.Contains(msgs[1], "alert-model") {
		t.Errorf("recovered alert should name the model, got: %s", msgs[1])
	}
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
	if cards[1].Fields["模型"] != "alert-model" {
		t.Errorf("recovered card 模型 field: got %q", cards[1].Fields["模型"])
	}

	events = listAlerts(t, ts, "")
	if len(events) != 2 {
		t.Fatalf("expected 2 alert events, got %d", len(events))
	}
	// Newest first: the recovered event leads.
	if events[0]["kind"].(string) != "recovered" {
		t.Errorf("expected newest event kind recovered, got %v", events[0]["kind"])
	}

	// The limit parameter is honored.
	if got := listAlerts(t, ts, "?limit=1"); len(got) != 1 {
		t.Errorf("limit=1: expected 1 event, got %d", len(got))
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

// TestAlertSendFailureRecorded verifies that a webhook answering 500 still
// records the alert event with sent_ok=false, and the failure is not retried
// on every subsequent round.
func TestAlertSendFailureRecorded(t *testing.T) {
	db := openTempDB(t)
	ts := newTestAPIServer(t, db)
	lark := newStubLarkServer(t)
	lark.setStatus(http.StatusInternalServerError)
	stubHub := newStubHubServer()
	defer stubHub.Close()

	putResp := doPut(t, ts.URL+"/api/settings", map[string]interface{}{
		"lark_webhook_url": lark.URL,
		"alert_enabled":    true,
	})
	putResp.Body.Close()

	endpointID := createProbedEndpoint(t, ts, "Fail Hub", stubHub.URL, "fail-model")
	stubHub.SetMode("error_503")

	runProbeRound(t, ts, endpointID)
	runProbeRound(t, ts, endpointID)

	events := listAlerts(t, ts, "")
	if len(events) != 1 {
		t.Fatalf("expected 1 alert event, got %d", len(events))
	}
	if events[0]["sent_ok"].(bool) != false {
		t.Error("expected sent_ok false when the webhook fails")
	}
	if events[0]["kind"].(string) != "down" {
		t.Errorf("expected kind down, got %v", events[0]["kind"])
	}

	// A failed send does not re-arm the alert: further failures stay quiet.
	runProbeRound(t, ts, endpointID)
	if got := listAlerts(t, ts, ""); len(got) != 1 {
		t.Fatalf("expected still 1 event after another failing round, got %d", len(got))
	}
}

// TestAlertRestartDoesNotRepeat simulates a service restart: a fresh server
// (fresh in-memory alert state) over the same database must not re-alert an
// endpoint whose down alert was already recorded.
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

	// First process: configure the webhook and drive the endpoint down.
	// WithSyncDiscovery keeps the hub-creation auto-sync inline (ticket 100).
	ts1 := httptest.NewServer(server.New(db, server.WithRateLimits(server.RateLimits{}), server.WithSyncDiscovery()))
	putResp := doPut(t, ts1.URL+"/api/settings", map[string]interface{}{
		"lark_webhook_url": lark.URL,
		"alert_enabled":    true,
	})
	putResp.Body.Close()
	endpointID := createProbedEndpoint(t, ts1, "Restart Hub", stubHub.URL, "restart-model")
	stubHub.SetMode("error_503")
	runProbeRound(t, ts1, endpointID)
	runProbeRound(t, ts1, endpointID)
	if got := len(lark.messages()); got != 1 {
		t.Fatalf("first process: expected 1 down alert, got %d", got)
	}
	ts1.Close()

	// "Restart": a brand-new server (fresh alert evaluator) over the same
	// database. Another failing round must not re-send the down alert.
	ts2 := httptest.NewServer(server.New(db, server.WithRateLimits(server.RateLimits{}), server.WithSyncDiscovery()))
	defer ts2.Close()
	runProbeRound(t, ts2, endpointID)
	if got := len(lark.messages()); got != 1 {
		t.Fatalf("after restart: expected still 1 message, got %d", got)
	}
	if got := listAlerts(t, ts2, ""); len(got) != 1 {
		t.Fatalf("after restart: expected 1 event, got %d", len(got))
	}

	// Recovery after the restart still fires exactly once.
	stubHub.SetMode("success")
	runProbeRound(t, ts2, endpointID)
	if got := len(lark.messages()); got != 2 {
		t.Fatalf("recovery after restart: expected 2 messages, got %d", got)
	}
}
