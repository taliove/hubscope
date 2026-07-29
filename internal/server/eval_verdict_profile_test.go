package server_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/taliove/hubscope/internal/store"
)

// TestVerdictNormalizationPipeline drives the v2 verdict profile end to end:
// exact/contains comparisons normalize both sides (trim, paired quotes,
// NFKC full/half-width folding, inner-whitespace collapse) while staying
// case-sensitive, regex is never normalized, and every result records the
// profile it was scored with (ADR 0008).
func TestVerdictNormalizationPipeline(t *testing.T) {
	ts, stub, db := setupEvalEnv(t)
	modelID := createEvalModel(t, ts.URL, stub.URL, "smart-model")
	suiteID := suiteIDByKey(t, ts.URL, "gsm8k")

	// Replace the seeded case set so the test controls every expectation.
	retireSuiteCases(t, db, suiteID)

	// Each entry stages one rule case plus the answer the stub will return.
	stages := []struct {
		marker   string
		mode     string
		expected string
		answer   string
		want     float64
	}{
		// Paired straight quotes around the answer are stripped.
		{"NORM-QUOTE", "exact", "e", `"e"`, 1},
		// Full-width letters fold to half-width under NFKC.
		{"NORM-FULLWIDTH", "exact", "e", "ｅ", 1},
		// Inner whitespace runs collapse to a single space; edges trim.
		{"NORM-SPACES", "exact", "a b c", "  a   b\tc  ", 1},
		// Case stays sensitive on purpose (instruction-following strictness).
		{"NORM-CASE", "exact", "Hello", "hello", 0},
		// Chinese corner brackets are paired quotes too.
		{"NORM-CNQUOTE", "exact", "天气", "「天气」", 1},
		// Contains runs through the same pipeline on both sides.
		{"NORM-CONTAINS", "contains", "artificial intelligence", "「artificial   intelligence」", 1},
		// Regex is the caliber itself: no normalization, full-width digits miss.
		{"NORM-REGEX", "regex", `^\d+$`, "４２", 0},
	}

	caseWant := map[int64]float64{}
	for _, s := range stages {
		resp := doPost(t, ts.URL+"/api/cases", map[string]interface{}{
			"suite_id":     suiteID,
			"prompt":       s.marker + ":请按要求作答",
			"verdict_type": "rule",
			"rule_config":  map[string]interface{}{"mode": s.mode, "expected": s.expected},
		})
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("create case %s: expected 201, got %d", s.marker, resp.StatusCode)
		}
		var env envelope
		_ = json.NewDecoder(resp.Body).Decode(&env)
		resp.Body.Close()
		var created map[string]interface{}
		_ = json.Unmarshal(env.Data, &created)
		caseWant[int64(created["id"].(float64))] = s.want
		stub.setAnswerSeq(s.marker, s.answer)
	}

	runID := triggerEval(t, ts.URL, suiteID, modelID)
	run := waitEvalDone(t, ts.URL, runID)
	if run["status"] != "done" {
		t.Fatalf("run status = %v, want done", run["status"])
	}

	results := resultsByModel(run, "smart-model")
	if len(results) != len(stages) {
		t.Fatalf("got %d results, want %d", len(results), len(stages))
	}
	for _, r := range results {
		caseID := int64(r["case_id"].(float64))
		want, ok := caseWant[caseID]
		if !ok {
			t.Fatalf("unexpected case %d in results", caseID)
		}
		if r["score"] != want {
			t.Errorf("case %d score = %v, want %v (answer detail: %v)", caseID, r["score"], want, r["verdict_detail"])
		}
		if r["verdict_profile"] != "v2" {
			t.Errorf("case %d verdict_profile = %v, want v2", caseID, r["verdict_profile"])
		}
	}
}

// TestVerdictProfileTrendBreak retags one campaign's results to the legacy
// v1 profile and asserts the trend exposes the caliber break on the v2 point
// (same shape as a suite-version break) while the v1 history keeps rendering.
func TestVerdictProfileTrendBreak(t *testing.T) {
	ts, stub, db := setupEvalEnv(t)
	modelID := createEvalModel(t, ts.URL, stub.URL, "smart-model")
	suiteID := suiteIDByKey(t, ts.URL, "gsm8k")
	// One custom exact-rule case: both campaigns score 100 on the raw scale.
	retireSuiteCases(t, db, suiteID)
	createRuleCase(t, ts.URL, suiteID, "PROFILE-TREND:请作答", "好的", nil)

	// Campaign 1 scores under what history calls the v1 caliber.
	run1 := triggerEval(t, ts.URL, suiteID, modelID)
	waitEvalDone(t, ts.URL, run1)
	c1 := campaignOfRun(t, ts.URL, run1)
	waitCampaignStatus(t, ts.URL, c1, store.CampaignStatusDone)
	if err := db.SetEvalRunVerdictProfile(run1, "v1"); err != nil {
		t.Fatalf("retag run %d to v1: %v", run1, err)
	}

	// Campaign 2 scores under the current v2 caliber, same suite version.
	run2 := triggerEval(t, ts.URL, suiteID, modelID)
	waitEvalDone(t, ts.URL, run2)
	c2 := campaignOfRun(t, ts.URL, run2)
	waitCampaignStatus(t, ts.URL, c2, store.CampaignStatusDone)

	// The legacy run keeps rendering with its v1 tag.
	history := getEvalRun(t, ts.URL, run1)
	for _, r := range resultsByModel(history, "smart-model") {
		if r["verdict_profile"] != "v1" {
			t.Errorf("legacy result verdict_profile = %v, want v1", r["verdict_profile"])
		}
		if r["score"] != 1.0 {
			t.Errorf("legacy result score = %v, want 1 (v1 data renders normally)", r["score"])
		}
	}

	trends := getCampaignTrends(t, ts.URL, c2, fmt.Sprintf("model=%d", modelID))
	suites := trendSuites(t, trends)
	if len(suites) != 1 {
		t.Fatalf("trend suites = %v, want exactly the basic suite", suites)
	}
	points := suites[0]["points"].([]interface{})
	if len(points) != 2 {
		t.Fatalf("trend points = %v, want 2", points)
	}
	p0 := points[0].(map[string]interface{})
	p1 := points[1].(map[string]interface{})

	if p0["verdict_profile"] != "v1" || p1["verdict_profile"] != "v2" {
		t.Errorf("point verdict profiles = [%v %v], want [v1 v2]", p0["verdict_profile"], p1["verdict_profile"])
	}
	// The break marker sits on the first point of the new caliber.
	if p0["profile_changed"] != false || p1["profile_changed"] != true {
		t.Errorf("profile_changed = [%v %v], want [false true]", p0["profile_changed"], p1["profile_changed"])
	}
	// The suite version never moved, so no question-bank break is flagged.
	if p0["version_changed"] != false || p1["version_changed"] != false {
		t.Errorf("version_changed = [%v %v], want [false false]", p0["version_changed"], p1["version_changed"])
	}
	// Both campaigns scored 100; the break is about comparability, not value.
	if p0["score"] != 100.0 || p1["score"] != 100.0 {
		t.Errorf("point scores = [%v %v], want [100 100]", p0["score"], p1["score"])
	}
}

// TestScoreDropSkippedAcrossVerdictProfiles stages a v1 baseline followed by
// a v2 campaign and asserts the cross-caliber comparison is skipped — no
// webhook message, one annotated score_drop_skipped event, a warn line on the
// run's task log — while a later same-caliber drop alerts normally.
func TestScoreDropSkippedAcrossVerdictProfiles(t *testing.T) {
	ts, stub, db := setupEvalEnv(t)
	lark := newStubLarkServer(t)
	enableScoreDropAlerts(t, ts, lark)

	modelID := createEvalModel(t, ts.URL, stub.URL, "caliber-model")
	suiteID := suiteIDByKey(t, ts.URL, "gsm8k")
	// One custom exact-rule case: campaign 1 aggregates 1.0; campaign 3
	// (model gone bad) aggregates 0.0 — the drop that alerts.
	retireSuiteCases(t, db, suiteID)
	createRuleCase(t, ts.URL, suiteID, "PROFILE-SKIP:请作答", "好的", nil)

	// Campaign 1: v1 baseline, aggregate 1.0.
	run1 := triggerEval(t, ts.URL, suiteID, modelID)
	waitEvalDone(t, ts.URL, run1)
	waitCampaignStatus(t, ts.URL, campaignOfRun(t, ts.URL, run1), store.CampaignStatusDone)
	if err := db.SetEvalRunVerdictProfile(run1, "v1"); err != nil {
		t.Fatalf("retag run %d to v1: %v", run1, err)
	}

	// Campaign 2: v2, same suite version. The comparison must be skipped even
	// though the caliber change alone could shift scores.
	run2 := triggerEval(t, ts.URL, suiteID, modelID)
	waitEvalDone(t, ts.URL, run2)
	waitCampaignStatus(t, ts.URL, campaignOfRun(t, ts.URL, run2), store.CampaignStatusDone)

	waitFor(t, "profile-skip annotated in task log", func() bool {
		for _, item := range taskItems(t, listTasks(t, ts.URL, "type=eval_run")) {
			if int64(item["entity_id"].(float64)) != run2 {
				continue
			}
			detail := getTaskDetail(t, ts.URL, int64(item["id"].(float64)))
			return countLogLines(taskLogs(t, detail), "warn", "判分口径已变更") >= 1
		}
		return false
	})

	skipped := alertEventsOfKind(t, ts, "score_drop_skipped")
	if len(skipped) != 1 {
		t.Fatalf("expected 1 score_drop_skipped event, got %d", len(skipped))
	}
	skipMsg, _ := skipped[0]["message"].(string)
	if !strings.Contains(skipMsg, "判分口径已变更") {
		t.Errorf("skip event message should mention 判分口径已变更, got: %s", skipMsg)
	}
	if skipped[0]["sent_ok"] != false {
		t.Error("skip event sent_ok should be false (nothing is sent for a skip)")
	}
	if got := len(lark.messages()); got != 0 {
		t.Errorf("cross-profile comparison: expected no webhook message, got %d", got)
	}
	if got := len(scoreDropEvents(t, ts)); got != 0 {
		t.Errorf("cross-profile comparison: expected no score_drop event, got %d", got)
	}

	// Campaign 3 stays on v2 and the model turns bad: a same-caliber drop
	// against campaign 2 alerts normally again.
	stub.markBad("caliber-model", true)
	run3 := triggerEval(t, ts.URL, suiteID, modelID)
	waitEvalDone(t, ts.URL, run3)
	waitCampaignStatus(t, ts.URL, campaignOfRun(t, ts.URL, run3), store.CampaignStatusDone)
	waitFor(t, "score_drop after profile stabilizes", func() bool {
		return len(lark.messages()) == 1
	})
	if !strings.Contains(lark.messages()[0], "caliber-model") {
		t.Errorf("post-skip alert should name the model, got: %s", lark.messages()[0])
	}
}
