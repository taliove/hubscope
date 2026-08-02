package server_test

import (
	"net/http"
	"strings"
	"testing"
)

// TestJudgeModelSettingRequiresKnownModel pins the GH #155 save-time guard
// (decision B): judge calls ride the evaluated model's hub, so a judge
// model that exists on no hub at all is a typo the settings API rejects
// with a clear 400 — instead of letting every judge case fail silently at
// eval time.
func TestJudgeModelSettingRequiresKnownModel(t *testing.T) {
	ts, stub, db := setupEvalEnv(t)
	createEvalModel(t, ts.URL, stub.URL, "smart-model")

	resp := doPut(t, ts.URL+"/api/settings", map[string]interface{}{
		"judge_model": "no-such-judge",
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("judge_model unknown on every hub: expected 400, got %d", resp.StatusCode)
	}

	// A judge registered on any hub passes the save-time guard.
	hub, err := db.CreateHub("judge-hub", "http://judge.test", "tok-judge-0000")
	if err != nil {
		t.Fatalf("create hub: %v", err)
	}
	if _, err := db.CreateModel(hub.ID, "known-judge", []string{"openai"}); err != nil {
		t.Fatalf("create judge model: %v", err)
	}
	resp = doPut(t, ts.URL+"/api/settings", map[string]interface{}{
		"judge_model": "known-judge",
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("judge_model present on a hub: expected 200, got %d", resp.StatusCode)
	}
}

// TestJudgeUnreachableWarnsOnTaskLog pins the GH #155 eval-time guard
// (decision B): when the configured judge is missing from the evaluated
// model's hub, the run's task log names the unreachable judge and hub up
// front — multi-hub deployments no longer learn about it from a wiped
// leaderboard.
func TestJudgeUnreachableWarnsOnTaskLog(t *testing.T) {
	ts, stub, db := setupEvalEnv(t)
	modelID := createEvalModel(t, ts.URL, stub.URL, "smart-model")
	suiteID := suiteIDByKey(t, ts.URL, "gsm8k")
	retireSuiteCases(t, db, suiteID)
	createJudgeCaseForTest(t, ts.URL, suiteID, "JUDGE-ROUTE:请回答")
	// The default judge (claude-opus-4-8) is registered on NO hub here.

	runID := triggerEval(t, ts.URL, suiteID, modelID)
	waitEvalDone(t, ts.URL, runID)

	var found bool
	items := taskItems(t, listTasks(t, ts.URL, "type=eval_run"))
	for _, item := range items {
		if int64(item["entity_id"].(float64)) != runID {
			continue
		}
		detail := getTaskDetail(t, ts.URL, int64(item["id"].(float64)))
		for _, line := range taskLogs(t, detail) {
			msg, _ := line["message"].(string)
			if strings.Contains(msg, "judge model") && strings.Contains(msg, "unreachable") {
				found = true
			}
		}
	}
	if !found {
		t.Errorf("run task log must name the unreachable judge up front (run %d)", runID)
	}
}
