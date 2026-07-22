package server_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// alertEventsOfKind returns only the alert events of the given kind.
func alertEventsOfKind(t *testing.T, ts *httptest.Server, kind string) []map[string]interface{} {
	t.Helper()
	var out []map[string]interface{}
	for _, e := range listAlerts(t, ts, "") {
		if e["kind"] == kind {
			out = append(out, e)
		}
	}
	return out
}

// scoreDropEvents returns only the score_drop alert events.
func scoreDropEvents(t *testing.T, ts *httptest.Server) []map[string]interface{} {
	t.Helper()
	return alertEventsOfKind(t, ts, "score_drop")
}

// enableScoreDropAlerts points the score-drop switch at the stub webhook.
func enableScoreDropAlerts(t *testing.T, ts *httptest.Server, lark *stubLarkServer) {
	t.Helper()
	putResp := doPut(t, ts.URL+"/api/settings", map[string]interface{}{
		"lark_webhook_url":         lark.URL,
		"score_drop_alert_enabled": true,
	})
	putResp.Body.Close()
	if putResp.StatusCode != http.StatusOK {
		t.Fatalf("put settings: expected 200, got %d", putResp.StatusCode)
	}
}

// campaignOfRun resolves the campaign wrapping the given run via its detail.
func campaignOfRun(t *testing.T, base string, runID int64) int64 {
	t.Helper()
	resp := doGet(t, fmt.Sprintf("%s/api/evals/%d", base, runID))
	defer resp.Body.Close()
	var env envelope
	_ = json.NewDecoder(resp.Body).Decode(&env)
	var run map[string]interface{}
	_ = json.Unmarshal(env.Data, &run)
	campaignID, ok := run["campaign_id"].(float64)
	if !ok || campaignID == 0 {
		t.Fatalf("run %d detail campaign_id missing: %v", runID, run)
	}
	return int64(campaignID)
}

// TestScoreDropAlert drives two campaigns whose per-model aggregate drops
// from 1.0 to 0.0 and asserts a score_drop alert is sent via the configured
// webhook and persisted with endpoint_id null — then that disabling the
// switch silences a later identical drop.
func TestScoreDropAlert(t *testing.T) {
	ts, stub, _ := setupEvalEnv(t)
	lark := newStubLarkServer(t)
	enableScoreDropAlerts(t, ts, lark)

	modelID := createEvalModel(t, ts.URL, stub.URL, "drop-model")
	suiteID := suiteIDByKey(t, ts.URL, "basic")

	// Campaign 1: everything correct (aggregate 1.0); no baseline, no alert.
	run1 := triggerEval(t, ts.URL, suiteID, modelID)
	waitEvalDone(t, ts.URL, run1)
	waitCampaignStatus(t, ts.URL, campaignOfRun(t, ts.URL, run1), "done")
	if got := len(lark.messages()); got != 0 {
		t.Fatalf("first campaign: expected no alerts without a baseline, got %d", got)
	}

	// Campaign 2: the model turns bad (aggregate 0.0) — a 1.0 drop fires once,
	// with per-suite drop and per-case change details in the message.
	stub.markBad("drop-model", true)
	run2 := triggerEval(t, ts.URL, suiteID, modelID)
	waitEvalDone(t, ts.URL, run2)
	waitCampaignStatus(t, ts.URL, campaignOfRun(t, ts.URL, run2), "done")

	waitFor(t, "score_drop webhook message", func() bool {
		return len(lark.messages()) == 1
	})
	msg := lark.messages()[0]
	for _, want := range []string{"drop-model", "基础指令遵循", "1.00", "0.00"} {
		if !strings.Contains(msg, want) {
			t.Errorf("score_drop message should contain %q, got: %s", want, msg)
		}
	}
	// Every case fell 1.00 -> 0.00, so the message carries case-level detail.
	if !strings.Contains(msg, "Case#") {
		t.Errorf("score_drop message should carry per-case details, got: %s", msg)
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

	// Campaign 3: the model recovers (rise, not a drop) — still just one alert.
	stub.markBad("drop-model", false)
	run3 := triggerEval(t, ts.URL, suiteID, modelID)
	waitEvalDone(t, ts.URL, run3)
	waitCampaignStatus(t, ts.URL, campaignOfRun(t, ts.URL, run3), "done")
	time.Sleep(200 * time.Millisecond)
	if got := len(lark.messages()); got != 1 {
		t.Fatalf("score rise: expected still 1 message, got %d", got)
	}

	// Disable the switch; an identical 1.0 -> 0.0 drop must stay silent and
	// record nothing.
	putResp := doPut(t, ts.URL+"/api/settings", map[string]interface{}{
		"score_drop_alert_enabled": false,
	})
	putResp.Body.Close()
	if putResp.StatusCode != http.StatusOK {
		t.Fatalf("disable score-drop alerts: expected 200, got %d", putResp.StatusCode)
	}

	stub.markBad("drop-model", true)
	run4 := triggerEval(t, ts.URL, suiteID, modelID)
	waitEvalDone(t, ts.URL, run4)

	// Give the (synchronous) post-campaign hook a chance to fire — it must not.
	waitCampaignStatus(t, ts.URL, campaignOfRun(t, ts.URL, run4), "done")
	time.Sleep(200 * time.Millisecond)
	if got := len(lark.messages()); got != 1 {
		t.Errorf("switch disabled: expected still 1 message, got %d", got)
	}
	if got := len(scoreDropEvents(t, ts)); got != 1 {
		t.Errorf("switch disabled: expected still 1 score_drop event, got %d", got)
	}
}

// TestScoreDropAlertSweepConsolidatesSuites runs two full-sweep campaigns and
// asserts the alert consolidates every dropped suite of the model into one
// message, while a suite without a drop stays out of it.
func TestScoreDropAlertSweepConsolidatesSuites(t *testing.T) {
	ts, stub, _ := setupEvalEnv(t)
	lark := newStubLarkServer(t)
	enableScoreDropAlerts(t, ts, lark)

	createEvalModel(t, ts.URL, stub.URL, "sweep-model")

	// Sweep 1: all-good baseline across every suite.
	sweep1 := triggerFullSweep(t, ts.URL)
	waitCampaignStatus(t, ts.URL, int64(sweep1["id"].(float64)), "done")
	if got := len(lark.messages()); got != 0 {
		t.Fatalf("first sweep: expected no alerts without a baseline, got %d", got)
	}

	// Sweep 2: the model turns bad. Every rule suite drops 1.0 -> 0.0; the
	// judge suite keeps its 0.75 stub verdicts and must not appear.
	stub.markBad("sweep-model", true)
	sweep2 := triggerFullSweep(t, ts.URL)
	waitCampaignStatus(t, ts.URL, int64(sweep2["id"].(float64)), "done")

	waitFor(t, "consolidated score_drop message", func() bool {
		return len(lark.messages()) == 1
	})
	msg := lark.messages()[0]
	for _, want := range []string{"sweep-model", "基础指令遵循", "推理数学", "代码能力"} {
		if !strings.Contains(msg, want) {
			t.Errorf("consolidated message should contain %q, got: %s", want, msg)
		}
	}
	if strings.Contains(msg, "中文能力") {
		t.Errorf("message must not list the judge suite (no drop there), got: %s", msg)
	}

	waitFor(t, "one consolidated score_drop event", func() bool {
		return len(scoreDropEvents(t, ts)) == 1
	})
}

// TestScoreDropSkippedAcrossSuiteVersions bumps the suite version between two
// campaigns and asserts the comparison is skipped — no webhook message — with
// the skip annotated both as an alert event and in the run's task log. A
// later campaign at the unchanged version alerts normally again.
func TestScoreDropSkippedAcrossSuiteVersions(t *testing.T) {
	ts, stub, _ := setupEvalEnv(t)
	lark := newStubLarkServer(t)
	enableScoreDropAlerts(t, ts, lark)

	modelID := createEvalModel(t, ts.URL, stub.URL, "ver-model")
	suiteID := suiteIDByKey(t, ts.URL, "basic")

	// Campaign 1 at suite version 1.
	run1 := triggerEval(t, ts.URL, suiteID, modelID)
	waitEvalDone(t, ts.URL, run1)
	waitCampaignStatus(t, ts.URL, campaignOfRun(t, ts.URL, run1), "done")

	// A new case bumps the suite to version 2.
	caseResp := doPost(t, ts.URL+"/api/cases", map[string]interface{}{
		"suite_id":     suiteID,
		"prompt":       "版本变更标记题:只回复 ok",
		"verdict_type": "rule",
		"rule_config":  map[string]interface{}{"mode": "exact", "expected": "ok"},
	})
	caseResp.Body.Close()
	if caseResp.StatusCode != http.StatusCreated {
		t.Fatalf("create case: expected 201, got %d", caseResp.StatusCode)
	}

	// Campaign 2 at version 2: the cross-version comparison must be skipped.
	run2 := triggerEval(t, ts.URL, suiteID, modelID)
	waitEvalDone(t, ts.URL, run2)
	waitCampaignStatus(t, ts.URL, campaignOfRun(t, ts.URL, run2), "done")

	waitFor(t, "version-skip annotated in task log", func() bool {
		for _, item := range taskItems(t, listTasks(t, ts.URL, "type=eval_run")) {
			if int64(item["entity_id"].(float64)) != run2 {
				continue
			}
			detail := getTaskDetail(t, ts.URL, int64(item["id"].(float64)))
			return countLogLines(taskLogs(t, detail), "warn", "题目已变更") >= 1
		}
		return false
	})

	// The run's task log carries the full annotation.
	for _, item := range taskItems(t, listTasks(t, ts.URL, "type=eval_run")) {
		if int64(item["entity_id"].(float64)) != run2 {
			continue
		}
		detail := getTaskDetail(t, ts.URL, int64(item["id"].(float64)))
		if got := countLogLines(taskLogs(t, detail), "warn", "分数不可比"); got < 1 {
			t.Errorf("task log should annotate 分数不可比, logs: %v", taskLogs(t, detail))
		}
	}

	// The skip is also recorded as an alert event; nothing hit the webhook.
	skipped := alertEventsOfKind(t, ts, "score_drop_skipped")
	if len(skipped) != 1 {
		t.Fatalf("expected 1 score_drop_skipped event, got %d", len(skipped))
	}
	skipMsg, _ := skipped[0]["message"].(string)
	for _, want := range []string{"题目已变更", "分数不可比"} {
		if !strings.Contains(skipMsg, want) {
			t.Errorf("skip event message should contain %q, got: %s", want, skipMsg)
		}
	}
	if skipped[0]["sent_ok"] != false {
		t.Error("skip event sent_ok should be false (nothing is sent for a skip)")
	}
	time.Sleep(200 * time.Millisecond)
	if got := len(lark.messages()); got != 0 {
		t.Errorf("cross-version comparison: expected no webhook message, got %d", got)
	}
	if got := len(scoreDropEvents(t, ts)); got != 0 {
		t.Errorf("cross-version comparison: expected no score_drop event, got %d", got)
	}

	// Campaign 3 at the same version 2 compares against campaign 2 and alerts.
	stub.markBad("ver-model", true)
	run3 := triggerEval(t, ts.URL, suiteID, modelID)
	waitEvalDone(t, ts.URL, run3)
	waitCampaignStatus(t, ts.URL, campaignOfRun(t, ts.URL, run3), "done")
	waitFor(t, "score_drop after version stabilizes", func() bool {
		return len(lark.messages()) == 1
	})
	if !strings.Contains(lark.messages()[0], "ver-model") {
		t.Errorf("post-skip alert should name the model, got: %s", lark.messages()[0])
	}
}

// TestScoreDropAlertCaseDetails drives a campaign pair where one case goes
// from scored to unjudged and another drops heavily, and asserts the alert
// message spells out both per-case changes while leaving stable cases out.
func TestScoreDropAlertCaseDetails(t *testing.T) {
	ts, stub, _ := setupEvalEnv(t)
	lark := newStubLarkServer(t)
	enableScoreDropAlerts(t, ts, lark)

	modelID := createEvalModel(t, ts.URL, stub.URL, "detail-model")
	suiteID := suiteIDByKey(t, ts.URL, "basic")

	// Replace the seeded case set with three scripted judge cases so the test
	// controls every score. Disabling bumps the version; both campaigns then
	// run at the same one.
	for _, c := range suiteByKey(t, ts.URL, "basic")["cases"].([]interface{}) {
		cm := c.(map[string]interface{})
		if enabled, _ := cm["enabled"].(bool); !enabled {
			continue
		}
		patchCase(t, ts.URL, int64(cm["id"].(float64)), map[string]interface{}{"enabled": false})
	}
	createJudgeCase := func(marker string) {
		t.Helper()
		resp := doPost(t, ts.URL+"/api/cases", map[string]interface{}{
			"suite_id":     suiteID,
			"prompt":       marker + ":请回答 1+1 等于几",
			"verdict_type": "judge",
			"rubric":       "评分标准:回答 2 得 1 分,否则 0 分。",
		})
		resp.Body.Close()
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("create judge case %s: expected 201, got %d", marker, resp.StatusCode)
		}
	}
	createJudgeCase("DROPTEST-ALPHA")
	createJudgeCase("DROPTEST-BETA")
	createJudgeCase("DROPTEST-GAMMA")

	// Round 1 all score 1.0; round 2: ALPHA's judge reply turns unparseable
	// (scored -> unjudged), BETA drops to 0.1, GAMMA stays put.
	stub.setJudgeSeq("DROPTEST-ALPHA", `{"score": 1.0, "reason": "ok"}`, "完全无法解析的垃圾回复")
	stub.setJudgeSeq("DROPTEST-BETA", `{"score": 1.0, "reason": "ok"}`, `{"score": 0.1, "reason": "bad"}`)
	stub.setJudgeSeq("DROPTEST-GAMMA", `{"score": 1.0, "reason": "ok"}`, `{"score": 1.0, "reason": "ok"}`)

	run1 := triggerEval(t, ts.URL, suiteID, modelID)
	waitEvalDone(t, ts.URL, run1)
	waitCampaignStatus(t, ts.URL, campaignOfRun(t, ts.URL, run1), "done")

	run2 := triggerEval(t, ts.URL, suiteID, modelID)
	waitEvalDone(t, ts.URL, run2)
	waitCampaignStatus(t, ts.URL, campaignOfRun(t, ts.URL, run2), "done")

	// Aggregate: 1.00 -> (0.10 + 1.00) / 2 = 0.55, a 0.45 drop beyond 0.2.
	waitFor(t, "case-detail score_drop message", func() bool {
		return len(lark.messages()) == 1
	})
	msg := lark.messages()[0]
	for _, want := range []string{"detail-model", "未判分", "DROPTEST-ALPHA", "1.00 → 0.10", "DROPTEST-BETA"} {
		if !strings.Contains(msg, want) {
			t.Errorf("case-detail message should contain %q, got: %s", want, msg)
		}
	}
	if strings.Contains(msg, "DROPTEST-GAMMA") {
		t.Errorf("stable case must stay out of the message, got: %s", msg)
	}
}
