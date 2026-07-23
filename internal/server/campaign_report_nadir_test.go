package server_test

import (
	"fmt"
	"math"
	"net/http"
	"testing"

	"github.com/taliove2009/hubscope/internal/store"
)

// createRuleCase posts one exact-match rule case to the suite and asserts
// HTTP 201. sampleCount is nil for cases that inherit the global default.
func createRuleCase(t *testing.T, base string, suiteID int64, prompt, expected string, sampleCount *int) {
	t.Helper()
	body := map[string]interface{}{
		"suite_id":     suiteID,
		"prompt":       prompt,
		"verdict_type": "rule",
		"rule_config":  map[string]interface{}{"mode": "exact", "expected": expected},
	}
	if sampleCount != nil {
		body["sample_count"] = *sampleCount
	}
	resp := doPost(t, base+"/api/cases", body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create case %q: expected 201, got %d", prompt, resp.StatusCode)
	}
}

// disableEnabledCases retires every enabled case of the suite via the API,
// leaving the suite's case set exactly under the test's control.
func disableEnabledCases(t *testing.T, base, suiteKey string) {
	t.Helper()
	for _, c := range suiteByKey(t, base, suiteKey)["cases"].([]interface{}) {
		cm := c.(map[string]interface{})
		if enabled, _ := cm["enabled"].(bool); enabled {
			patchCase(t, base, int64(cm["id"].(float64)), map[string]interface{}{"enabled": false})
		}
	}
}

// rowCell extracts one suite's progress cell from a leaderboard row.
func rowCell(t *testing.T, row map[string]interface{}, suiteKey string) map[string]interface{} {
	t.Helper()
	cells, ok := row["cells"].([]interface{})
	if !ok {
		t.Fatalf("row cells missing or wrong type: %v", row)
	}
	for _, c := range cells {
		cell := c.(map[string]interface{})
		if cell["suite_key"] == suiteKey {
			return cell
		}
	}
	t.Fatalf("cell for suite %q missing: %v", suiteKey, cells)
	return nil
}

// suiteScoreOf reads a row's per-suite score; ok=false when the key is
// absent or null (unscored suite).
func suiteScoreOf(row map[string]interface{}, suiteKey string) (float64, bool) {
	scores, ok := row["suite_scores"].(map[string]interface{})
	if !ok {
		return 0, false
	}
	raw, present := scores[suiteKey]
	if !present || raw == nil {
		return 0, false
	}
	return raw.(float64), true
}

// TestCampaignReportNadirNormalization pins the ADR-0009 report-layer
// scoring: per-suite scores normalize as (mean - nadir) / (1 - nadir) * 100
// clamped to [0, 100] using the run's nadir snapshot, nadir=0 degenerates to
// the legacy raw-mean caliber, the board returns every dimension's score
// side by side, unscored suites drop out of the total (numerator and
// denominator alike), and every score carries its coverage/sample
// confidence markers.
func TestCampaignReportNadirNormalization(t *testing.T) {
	ts, stub, _ := setupEvalEnv(t)
	smartID := createEvalModel(t, ts.URL, stub.URL, "smart-model")
	createEvalModel(t, ts.URL, stub.URL, "dumb-model")
	createEvalModel(t, ts.URL, stub.URL, "broken-model")
	stub.markBroken("broken-model", true)

	// cap_knowledge is seeded with nadir 0.25 (random-guess floor of its
	// four-option cases). Replace its case set with four exact-rule cases the
	// stub answers deterministically: the smart model defaults to "好的",
	// the dumb model to "随便说点什么", so smart judges 3/4 (mean 0.75) and
	// dumb exactly 1/4 (mean 0.25 — the nadir boundary itself).
	knowledgeID := suiteIDByKey(t, ts.URL, "cap_knowledge")
	disableEnabledCases(t, ts.URL, "cap_knowledge")
	two := 2
	createRuleCase(t, ts.URL, knowledgeID, "NADIR-K1:请作答", "好的", &two)
	createRuleCase(t, ts.URL, knowledgeID, "NADIR-K2:请作答", "好的", nil)
	createRuleCase(t, ts.URL, knowledgeID, "NADIR-K3:请作答", "好的", nil)
	createRuleCase(t, ts.URL, knowledgeID, "NADIR-K4:请作答", "随便说点什么", nil)

	// cap_instruction is seeded with nadir 0: its scores must stay on the
	// legacy raw-mean caliber. Two cases, smart judges 1/2 (mean 0.5) — 50 on
	// the raw scale, where a nadir of 0.25 would have produced 33.3.
	instructionID := suiteIDByKey(t, ts.URL, "cap_instruction")
	disableEnabledCases(t, ts.URL, "cap_instruction")
	createRuleCase(t, ts.URL, instructionID, "NADIR-I1:请作答", "好的", nil)
	createRuleCase(t, ts.URL, instructionID, "NADIR-I2:请作答", "站住", nil)

	campaign := triggerFullSweep(t, ts.URL)
	campaignID := int64(campaign["id"].(float64))
	waitCampaignStatus(t, ts.URL, campaignID, store.CampaignStatusDone)

	report := getCampaignReport(t, ts.URL, campaignID, "")
	rows := reportRows(t, report)
	smart := rowByModel(t, rows, "smart-model")
	dumb := rowByModel(t, rows, "dumb-model")
	broken := rowByModel(t, rows, "broken-model")

	// Nadir formula: (0.75 - 0.25) / (1 - 0.25) * 100 = 66.67 for smart;
	// the dumb model sits exactly on the nadir boundary and scores 0.
	knowledgeWant := (0.75 - 0.25) / (1 - 0.25) * 100
	if got, ok := suiteScoreOf(smart, "cap_knowledge"); !ok || math.Abs(got-knowledgeWant) > 1e-9 {
		t.Errorf("smart cap_knowledge = %v (ok=%v), want %v", got, ok, knowledgeWant)
	}
	if got, ok := suiteScoreOf(dumb, "cap_knowledge"); !ok || got != 0 {
		t.Errorf("dumb cap_knowledge = %v (ok=%v), want exactly 0 at the nadir boundary", got, ok)
	}

	// nadir=0 degenerates to the legacy caliber: raw mean x 100.
	if got, ok := suiteScoreOf(smart, "cap_instruction"); !ok || got != 50 {
		t.Errorf("smart cap_instruction = %v (ok=%v), want 50 (raw-mean caliber)", got, ok)
	}
	if got, ok := suiteScoreOf(dumb, "cap_instruction"); !ok || got != 0 {
		t.Errorf("dumb cap_instruction = %v (ok=%v), want 0", got, ok)
	}

	// Dimension scores side by side: every covered suite key is present on
	// the row, not just behind a suite switch.
	suiteKeys := []string{"cap_instruction", "cap_reasoning", "cap_coding", "cap_knowledge", "cap_language"}
	for _, key := range suiteKeys {
		if _, ok := suiteScoreOf(smart, key); !ok {
			t.Errorf("smart row missing dimension score for %q: %v", key, smart["suite_scores"])
		}
	}

	// The broken model judged nothing: every dimension is null and the
	// total is null (unscored suites never count as zero).
	for _, key := range suiteKeys {
		if got, ok := suiteScoreOf(broken, key); ok {
			t.Errorf("broken %q = %v, want unscored (null)", key, got)
		}
	}
	if broken["total_score"] != nil {
		t.Errorf("broken total_score = %v, want null (nothing scored)", broken["total_score"])
	}
	assertRowTotals(t, report)

	// Confidence markers: coverage plus judged-sample count per cell. K1
	// samples twice, the other three cases once each: 5 judged samples.
	smartCell := rowCell(t, smart, "cap_knowledge")
	if smartCell["judged_cases"] != 4.0 || smartCell["expected_cases"] != 4.0 {
		t.Errorf("smart knowledge cell coverage = %v/%v, want 4/4", smartCell["judged_cases"], smartCell["expected_cases"])
	}
	if smartCell["samples"] != 5.0 {
		t.Errorf("smart knowledge cell samples = %v, want 5 (2+1+1+1)", smartCell["samples"])
	}
	brokenCell := rowCell(t, broken, "cap_knowledge")
	if brokenCell["judged_cases"] != 0.0 || brokenCell["expected_cases"] != 4.0 || brokenCell["samples"] != 0.0 {
		t.Errorf("broken knowledge cell = %v, want judged 0/4 with 0 samples", brokenCell)
	}

	// The drill-down trend speaks the same normalized scale as the board.
	trends := getCampaignTrends(t, ts.URL, campaignID, fmt.Sprintf("model=%d", smartID))
	trendKnowledge := 0.0
	found := false
	for _, suite := range trendSuites(t, trends) {
		if suite["key"] != "cap_knowledge" {
			continue
		}
		points := suite["points"].([]interface{})
		if len(points) != 1 {
			t.Fatalf("knowledge trend points = %v, want 1", points)
		}
		trendKnowledge = points[0].(map[string]interface{})["score"].(float64)
		found = true
	}
	if !found {
		t.Fatalf("trend missing cap_knowledge series: %v", trends["suites"])
	}
	if math.Abs(trendKnowledge-knowledgeWant) > 1e-9 {
		t.Errorf("knowledge trend score = %v, want normalized %v", trendKnowledge, knowledgeWant)
	}

	// The run-level aggregate speaks the same normalized caliber on the eval
	// API's 0~1 wire scale: knowledge averages the raw case scores (smart
	// 1,1,1,0; dumb 0,0,0,1; broken unscored) to 0.5, normalized
	// (0.5 - 0.25) / (1 - 0.25) = 1/3; instruction stays the legacy raw mean
	// 0.25 under nadir 0.
	runsByKey := map[string]map[string]interface{}{}
	for _, run := range campaignRuns(t, getCampaign(t, ts.URL, campaignID)) {
		key := suiteByID(t, ts.URL, int64(run["suite_id"].(float64)))["key"].(string)
		runsByKey[key] = run
	}
	if got := runsByKey["cap_knowledge"]["score"].(float64); math.Abs(got-1.0/3.0) > 1e-9 {
		t.Errorf("knowledge run aggregate = %v, want 1/3 (nadir-normalized)", got)
	}
	if got := runsByKey["cap_instruction"]["score"].(float64); got != 0.25 {
		t.Errorf("instruction run aggregate = %v, want 0.25 (raw-mean caliber)", got)
	}

	// Campaign 2: cap_reasoning loses its entire case set, so its run
	// records no results and the suite drops out of every total. If unscored
	// suites were diluted in as zeros, the total would sag by a fifth.
	disableEnabledCases(t, ts.URL, "cap_reasoning")
	second := triggerFullSweep(t, ts.URL)
	secondID := int64(second["id"].(float64))
	waitCampaignStatus(t, ts.URL, secondID, store.CampaignStatusDone)

	report2 := getCampaignReport(t, ts.URL, secondID, "")
	smart2 := rowByModel(t, reportRows(t, report2), "smart-model")
	if got, ok := suiteScoreOf(smart2, "cap_reasoning"); ok {
		t.Errorf("cap_reasoning without results must be unscored, got %v", got)
	}
	var sum, n float64
	for _, key := range []string{"cap_instruction", "cap_coding", "cap_knowledge", "cap_language"} {
		got, ok := suiteScoreOf(smart2, key)
		if !ok {
			t.Fatalf("smart campaign-2 row missing scored suite %q: %v", key, smart2["suite_scores"])
		}
		sum += got
		n++
	}
	total2 := totalOf(t, smart2)
	if want := sum / n; math.Abs(total2-want) > 1e-9 {
		t.Errorf("smart campaign-2 total = %v, want mean of scored suites %v", total2, want)
	}
	if diluted := sum / (n + 1); math.Abs(total2-diluted) < 1e-9 {
		t.Errorf("smart campaign-2 total = %v matches the diluted %v — unscored suite counted as zero", total2, diluted)
	}
}

// TestCampaignReportDeltaProfileChanged pins the ADR-0008 baseline rule
// (ticket 49 report-layer follow-up): adjacent batches scored under
// different verdict profiles are as incomparable as a question-bank change —
// the baseline carries the profile_changed reason and every delta stays
// null; once the profile stabilizes, deltas flow again.
func TestCampaignReportDeltaProfileChanged(t *testing.T) {
	ts, stub, db := setupEvalEnv(t)
	modelID := createEvalModel(t, ts.URL, stub.URL, "smart-model")
	suiteID := suiteIDByKey(t, ts.URL, "basic")

	// Batch 1 scores under what history calls the v1 caliber.
	run1 := triggerEval(t, ts.URL, suiteID, modelID)
	waitEvalDone(t, ts.URL, run1)
	c1 := campaignOfRun(t, ts.URL, run1)
	waitCampaignStatus(t, ts.URL, c1, store.CampaignStatusDone)
	if err := db.SetEvalRunVerdictProfile(run1, "v1"); err != nil {
		t.Fatalf("retag run %d to v1: %v", run1, err)
	}

	// Batch 2: same question-bank version, current v2 caliber. The version
	// check alone would call this comparable; the profile check must not.
	run2 := triggerEval(t, ts.URL, suiteID, modelID)
	waitEvalDone(t, ts.URL, run2)
	c2 := campaignOfRun(t, ts.URL, run2)
	waitCampaignStatus(t, ts.URL, c2, store.CampaignStatusDone)

	report2 := getCampaignReport(t, ts.URL, c2, "")
	baseline2 := reportBaseline(t, report2)
	if baseline2 == nil {
		t.Fatalf("batch 2 baseline missing: %v", report2)
	}
	if int64(baseline2["campaign_id"].(float64)) != c1 {
		t.Errorf("baseline campaign = %v, want batch %d", baseline2["campaign_id"], c1)
	}
	if baseline2["comparable"] != false {
		t.Errorf("cross-profile baseline comparable = %v, want false", baseline2)
	}
	if baseline2["reason"] != "profile_changed" {
		t.Errorf("baseline reason = %v, want profile_changed", baseline2["reason"])
	}
	for _, row := range reportRows(t, report2) {
		if d := row["total_delta"]; d != nil {
			t.Errorf("cross-profile row %v total_delta = %v, want null", row["model_id"], d)
		}
	}

	// Batch 3 stays on v2: same caliber as batch 2, so the comparison flows
	// again and the steady model shows a numeric (zero) delta.
	run3 := triggerEval(t, ts.URL, suiteID, modelID)
	waitEvalDone(t, ts.URL, run3)
	c3 := campaignOfRun(t, ts.URL, run3)
	waitCampaignStatus(t, ts.URL, c3, store.CampaignStatusDone)

	report3 := getCampaignReport(t, ts.URL, c3, "")
	baseline3 := reportBaseline(t, report3)
	if baseline3 == nil {
		t.Fatalf("batch 3 baseline missing: %v", report3)
	}
	if baseline3["comparable"] != true {
		t.Errorf("same-profile baseline comparable = %v, want true", baseline3)
	}
	smart3 := rowByModel(t, reportRows(t, report3), "smart-model")
	if _, ok := smart3["total_delta"].(float64); !ok {
		t.Errorf("same-profile total_delta = %v, want a number", smart3["total_delta"])
	}
}
