package server_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// scoreDropEvents returns only the score_drop alert events.
func scoreDropEvents(t *testing.T, ts *httptest.Server) []map[string]interface{} {
	t.Helper()
	var out []map[string]interface{}
	for _, e := range listAlerts(t, ts, "") {
		if e["kind"] == "score_drop" {
			out = append(out, e)
		}
	}
	return out
}

// TestScoreDropAlert drives two eval rounds whose per-model aggregate drops
// from 1.0 to 0.0 and asserts a score_drop alert is sent via the configured
// webhook and persisted with endpoint_id null — then that disabling the
// switch silences a later identical drop.
func TestScoreDropAlert(t *testing.T) {
	ts, stub, _ := setupEvalEnv(t)
	lark := newStubLarkServer(t)

	putResp := doPut(t, ts.URL+"/api/settings", map[string]interface{}{
		"lark_webhook_url":         lark.URL,
		"score_drop_alert_enabled": true,
	})
	putResp.Body.Close()
	if putResp.StatusCode != http.StatusOK {
		t.Fatalf("put settings: expected 200, got %d", putResp.StatusCode)
	}

	modelID := createEvalModel(t, ts.URL, stub.URL, "drop-model")
	suiteID := suiteIDByKey(t, ts.URL, "basic")

	// Round 1: everything correct (aggregate 1.0); no previous run, no alert.
	run1 := triggerEval(t, ts.URL, suiteID, modelID)
	waitEvalDone(t, ts.URL, run1)
	if got := len(lark.messages()); got != 0 {
		t.Fatalf("first run: expected no alerts without a baseline, got %d", got)
	}

	// Round 2: the model turns bad (aggregate 0.0) — a 1.0 drop fires once.
	stub.markBad("drop-model", true)
	run2 := triggerEval(t, ts.URL, suiteID, modelID)
	waitEvalDone(t, ts.URL, run2)

	waitFor(t, "score_drop webhook message", func() bool {
		return len(lark.messages()) == 1
	})
	msg := lark.messages()[0]
	for _, want := range []string{"drop-model", "基础指令遵循", "1.00", "0.00"} {
		if !strings.Contains(msg, want) {
			t.Errorf("score_drop message should contain %q, got: %s", want, msg)
		}
	}

	waitFor(t, "score_drop event persisted", func() bool {
		return len(scoreDropEvents(t, ts)) == 1
	})
	event := scoreDropEvents(t, ts)[0]
	if event["endpoint_id"] != nil {
		t.Errorf("score_drop event endpoint_id = %v, want null", event["endpoint_id"])
	}
	if event["sent_ok"] != true {
		t.Error("score_drop event sent_ok should be true")
	}
	if event["message"] != msg {
		t.Errorf("event message should match the webhook text:\nevent: %v\nsent:  %v", event["message"], msg)
	}

	// Round 3: the model recovers (rise, not a drop) — still just one alert.
	stub.markBad("drop-model", false)
	run3 := triggerEval(t, ts.URL, suiteID, modelID)
	waitEvalDone(t, ts.URL, run3)
	if got := len(lark.messages()); got != 1 {
		t.Fatalf("score rise: expected still 1 message, got %d", got)
	}

	// Disable the switch; an identical 1.0 -> 0.0 drop must stay silent and
	// record nothing.
	putResp = doPut(t, ts.URL+"/api/settings", map[string]interface{}{
		"score_drop_alert_enabled": false,
	})
	putResp.Body.Close()
	if putResp.StatusCode != http.StatusOK {
		t.Fatalf("disable score-drop alerts: expected 200, got %d", putResp.StatusCode)
	}

	stub.markBad("drop-model", true)
	run4 := triggerEval(t, ts.URL, suiteID, modelID)
	waitEvalDone(t, ts.URL, run4)

	// Give the (synchronous) post-run hook a chance to fire — it must not.
	waitFor(t, "round 4 fully settled", func() bool {
		runs := listEvalRuns(t, ts.URL)
		return len(runs) == 4 && runs[0]["status"] == "done"
	})
	if got := len(lark.messages()); got != 1 {
		t.Errorf("switch disabled: expected still 1 message, got %d", got)
	}
	if got := len(scoreDropEvents(t, ts)); got != 1 {
		t.Errorf("switch disabled: expected still 1 score_drop event, got %d", got)
	}
}
