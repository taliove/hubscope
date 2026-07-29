package server_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// These tests cover ticket 100 (the "send test" button for the Lark alert
// channel) at the W1 seam: black-box HTTP against a stub webhook plus a real
// SQLite temp DB. Three decisions pin the behavior: the test targets the
// address in the request body (not the saved setting), every attempt records
// an alert_events row with kind="test" (endpoint_id NULL, success or
// failure), and the alert_enabled switch does not gate the manual test.

// postTestLark issues one POST /api/settings/test-lark and returns the raw
// response.
func postTestLark(t *testing.T, tsURL string, body interface{}) *http.Response {
	t.Helper()
	return doPost(t, tsURL+"/api/settings/test-lark", body)
}

// decodeTestLarkResult requires a 200 and returns the data payload.
func decodeTestLarkResult(t *testing.T, resp *http.Response) map[string]interface{} {
	t.Helper()
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("test-lark: expected 200, got %d", resp.StatusCode)
	}
	var env envelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode test-lark response: %v", err)
	}
	var data map[string]interface{}
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("unmarshal test-lark data: %v", err)
	}
	return data
}

// TestTestLarkSuccessSendsAndRecords verifies a working webhook answers
// sent_ok=true, delivers the fixed test text, and records a kind="test"
// event with a NULL endpoint_id.
func TestTestLarkSuccessSendsAndRecords(t *testing.T) {
	db := openTempDB(t)
	ts := newTestAPIServer(t, db)
	lark := newStubLarkServer(t)

	data := decodeTestLarkResult(t, postTestLark(t, ts.URL, map[string]interface{}{
		"webhook_url": lark.URL,
	}))
	if data["sent_ok"].(bool) != true {
		t.Errorf("expected sent_ok true, got %v", data["sent_ok"])
	}
	if data["error"] != nil {
		t.Errorf("expected null error, got %v", data["error"])
	}

	msgs := lark.messages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 lark message, got %d", len(msgs))
	}
	if !strings.Contains(msgs[0], "测试消息") {
		t.Errorf("test message should carry the fixed wording, got: %s", msgs[0])
	}

	// The test message goes out as a turquoise-header interactive card
	// (ticket 101) while the recorded event below stays plain text.
	cards := lark.cards()
	if len(cards) != 1 {
		t.Fatalf("expected 1 card, got %d", len(cards))
	}
	if cards[0].Template != "turquoise" {
		t.Errorf("test card template: expected turquoise, got %q", cards[0].Template)
	}
	if cards[0].Title != "测试消息 · HubScope" {
		t.Errorf("test card title: got %q", cards[0].Title)
	}

	events := listAlerts(t, ts, "")
	if len(events) != 1 {
		t.Fatalf("expected 1 alert event, got %d", len(events))
	}
	if events[0]["kind"].(string) != "test" {
		t.Errorf("expected kind test, got %v", events[0]["kind"])
	}
	if events[0]["endpoint_id"] != nil {
		t.Errorf("test event must have a null endpoint_id, got %v", events[0]["endpoint_id"])
	}
	if events[0]["sent_ok"].(bool) != true {
		t.Error("expected sent_ok true on the recorded event")
	}
	if !strings.Contains(events[0]["message"].(string), "测试消息") {
		t.Errorf("event message should carry the fixed wording, got: %v", events[0]["message"])
	}
}

// TestTestLarkFailureRecorded verifies a webhook answering 500 reports
// sent_ok=false with a reason, still records the event (sent_ok=false), and
// never echoes the webhook URL back (W6: the URL carries the bot token).
func TestTestLarkFailureRecorded(t *testing.T) {
	db := openTempDB(t)
	ts := newTestAPIServer(t, db)
	lark := newStubLarkServer(t)
	lark.setStatus(http.StatusInternalServerError)

	data := decodeTestLarkResult(t, postTestLark(t, ts.URL, map[string]interface{}{
		"webhook_url": lark.URL,
	}))
	if data["sent_ok"].(bool) != false {
		t.Errorf("expected sent_ok false, got %v", data["sent_ok"])
	}
	errText, ok := data["error"].(string)
	if !ok || errText == "" {
		t.Fatalf("expected a non-empty error string, got %v", data["error"])
	}
	if strings.Contains(errText, lark.URL) {
		t.Errorf("error must never echo the webhook URL, got: %s", errText)
	}

	events := listAlerts(t, ts, "")
	if len(events) != 1 {
		t.Fatalf("expected 1 alert event, got %d", len(events))
	}
	if events[0]["kind"].(string) != "test" {
		t.Errorf("expected kind test, got %v", events[0]["kind"])
	}
	if events[0]["sent_ok"].(bool) != false {
		t.Error("expected sent_ok false on the recorded event")
	}

	// The audit trail records the action and outcome but never the webhook
	// URL (W6) — the URL carries the bot token.
	page := fetchAuditLogs(t, ts.URL, "?action=settings.test_lark")
	items := auditItems(t, page)
	if len(items) != 1 {
		t.Fatalf("expected 1 settings.test_lark audit entry, got %d", len(items))
	}
	blob, err := json.Marshal(items[0])
	if err != nil {
		t.Fatalf("marshal audit item: %v", err)
	}
	if strings.Contains(string(blob), lark.URL) {
		t.Errorf("audit entry must never contain the webhook URL: %s", blob)
	}
}

// TestTestLarkValidation rejects a missing, empty, or non-http(s) webhook
// address with 400 and records nothing. Anonymous callers get 401.
func TestTestLarkValidation(t *testing.T) {
	db := openTempDB(t)
	ts := newTestAPIServer(t, db)

	bad := []interface{}{
		map[string]interface{}{},
		map[string]interface{}{"webhook_url": ""},
		map[string]interface{}{"webhook_url": "   "},
		map[string]interface{}{"webhook_url": "not-a-url"},
		map[string]interface{}{"webhook_url": "ftp://example.com/hook"},
		map[string]interface{}{"webhook_url": "//example.com/hook"},
	}
	for _, body := range bad {
		resp := postTestLark(t, ts.URL, body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("body %v: expected 400, got %d", body, resp.StatusCode)
		}
	}
	if got := listAlerts(t, ts, ""); len(got) != 0 {
		t.Fatalf("rejected requests must record no events, got %d", len(got))
	}

	// Anonymous POST is rejected at the session gate.
	req, _ := http.NewRequest("POST", ts.URL+"/api/settings/test-lark",
		strings.NewReader(`{"webhook_url":"https://open.feishu.cn/example-hook"}`))
	req.Header.Set("Content-Type", "application/json")
	anonResp, err := (&http.Client{}).Do(req)
	if err != nil {
		t.Fatalf("anonymous POST: %v", err)
	}
	anonResp.Body.Close()
	if anonResp.StatusCode != http.StatusUnauthorized {
		t.Errorf("anonymous POST: expected 401, got %d", anonResp.StatusCode)
	}
}

// TestTestLarkTargetsBodyAddressAndIgnoresSwitch pins the two form-side
// decisions: the test goes to the address in the request body even when no
// webhook is saved, and alert_enabled=false does not gate the manual test
// (the switch governs automatic alerts only).
func TestTestLarkTargetsBodyAddressAndIgnoresSwitch(t *testing.T) {
	db := openTempDB(t)
	ts := newTestAPIServer(t, db)
	lark := newStubLarkServer(t)

	putResp := doPut(t, ts.URL+"/api/settings", map[string]interface{}{
		"alert_enabled": false,
	})
	putResp.Body.Close()
	if putResp.StatusCode != http.StatusOK {
		t.Fatalf("put settings: expected 200, got %d", putResp.StatusCode)
	}

	data := decodeTestLarkResult(t, postTestLark(t, ts.URL, map[string]interface{}{
		"webhook_url": lark.URL,
	}))
	if data["sent_ok"].(bool) != true {
		t.Errorf("alerts disabled: test send should still succeed, got %v", data)
	}
	if got := len(lark.messages()); got != 1 {
		t.Fatalf("expected 1 lark message, got %d", got)
	}
	events := listAlerts(t, ts, "")
	if len(events) != 1 || events[0]["kind"].(string) != "test" {
		t.Fatalf("expected 1 kind=test event, got %v", events)
	}
}

// TestTestLarkDoesNotPolluteAlertState is the ticket-100 risk-4 regression:
// kind="test" events must stay out of the down/recovered lazy state rebuild
// (LatestDownRecoveryEvent filters by a down/recovered whitelist). With test
// events in the log, the endpoint lifecycle still fires exactly one down
// alert and one recovered notice.
func TestTestLarkDoesNotPolluteAlertState(t *testing.T) {
	db := openTempDB(t)
	ts := newTestAPIServer(t, db)
	lark := newStubLarkServer(t)
	stubHub := newStubHubServer()
	defer stubHub.Close()

	putResp := doPut(t, ts.URL+"/api/settings", map[string]interface{}{
		"lark_webhook_url": lark.URL,
		"alert_enabled":    true,
	})
	putResp.Body.Close()

	// Two test events land in the log before any probe transition.
	for i := 0; i < 2; i++ {
		data := decodeTestLarkResult(t, postTestLark(t, ts.URL, map[string]interface{}{
			"webhook_url": lark.URL,
		}))
		if data["sent_ok"].(bool) != true {
			t.Fatalf("test send %d failed: %v", i, data)
		}
	}

	endpointID := createProbedEndpoint(t, ts, "Test Hub", stubHub.URL, "test-model")
	stubHub.SetMode("error_503")

	// Crossing the failure threshold fires exactly one down alert: the test
	// events must not read as an open (or closed) outage.
	runProbeRound(t, ts, endpointID)
	runProbeRound(t, ts, endpointID)
	msgs := lark.messages()
	// 2 test messages + 1 down alert.
	if len(msgs) != 3 {
		t.Fatalf("after crossing threshold: expected 3 messages (2 test + 1 down), got %d", len(msgs))
	}
	if !strings.Contains(msgs[2], "test-model") {
		t.Errorf("down alert should name the model, got: %s", msgs[2])
	}

	// Recovery fires exactly once as well.
	stubHub.SetMode("success")
	runProbeRound(t, ts, endpointID)
	if got := len(lark.messages()); got != 4 {
		t.Fatalf("after recovery: expected 4 messages, got %d", got)
	}

	events := listAlerts(t, ts, "")
	if len(events) != 4 {
		t.Fatalf("expected 4 events (2 test + down + recovered), got %d", len(events))
	}
	// Newest first: recovered, down, then the two test events.
	if events[0]["kind"].(string) != "recovered" || events[1]["kind"].(string) != "down" {
		t.Errorf("expected recovered+down leading, got %v / %v", events[0]["kind"], events[1]["kind"])
	}
	if events[2]["kind"].(string) != "test" || events[3]["kind"].(string) != "test" {
		t.Errorf("expected trailing test events, got %v / %v", events[2]["kind"], events[3]["kind"])
	}
}
