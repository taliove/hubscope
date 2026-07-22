package server_test

import (
	"math"
	"testing"

	"github.com/taliove2009/hubscope/internal/store"
)

// reportBaseline extracts the baseline block of a report payload; returns
// nil when the API reports there is no previous done campaign.
func reportBaseline(t *testing.T, report map[string]interface{}) map[string]interface{} {
	t.Helper()
	raw, ok := report["baseline"]
	if !ok {
		t.Fatalf("report baseline key missing: %v", report)
	}
	if raw == nil {
		return nil
	}
	baseline, ok := raw.(map[string]interface{})
	if !ok {
		t.Fatalf("report baseline wrong type: %v", raw)
	}
	return baseline
}

// rowByModel locates a leaderboard row by model id.
func rowByModel(t *testing.T, rows []map[string]interface{}, modelID string) map[string]interface{} {
	t.Helper()
	for _, row := range rows {
		if row["model_id"] == modelID {
			return row
		}
	}
	t.Fatalf("model %q not on leaderboard: %v", modelID, rows)
	return nil
}

// totalOf reads a row's total_score, requiring a number.
func totalOf(t *testing.T, row map[string]interface{}) float64 {
	t.Helper()
	value, ok := row["total_score"].(float64)
	if !ok {
		t.Fatalf("row total_score = %v, want a number: %v", row["total_score"], row)
	}
	return value
}

// TestCampaignReportTotalDelta covers the previous-batch delta contract
// (ticket 45): the first batch has no baseline and no deltas; the next batch
// carries per-row total deltas against it (models new to the batch excepted);
// a suite version bump in between marks the baseline incomparable and
// suppresses every delta (ADR 0007: a question-bank change must never read
// as a model change).
func TestCampaignReportTotalDelta(t *testing.T) {
	ts, stub, _ := setupEvalEnv(t)
	createEvalModel(t, ts.URL, stub.URL, "smart-model")

	// Batch 1: no earlier done campaign exists, so there is no baseline and
	// every row's delta is null.
	first := triggerFullSweep(t, ts.URL)
	firstID := int64(first["id"].(float64))
	waitCampaignStatus(t, ts.URL, firstID, store.CampaignStatusDone)

	report1 := getCampaignReport(t, ts.URL, firstID, "")
	if baseline := reportBaseline(t, report1); baseline != nil {
		t.Errorf("first batch baseline = %v, want null", baseline)
	}
	smart1 := rowByModel(t, reportRows(t, report1), "smart-model")
	if delta, ok := smart1["total_delta"]; !ok || delta != nil {
		t.Errorf("first batch total_delta = %v (key present=%v), want null", delta, ok)
	}
	total1 := totalOf(t, smart1)

	// Batch 2: the smart model degrades and a fresh model joins. The fresh
	// model has no baseline score and must carry a null delta while the
	// degraded model shows exactly its total-score drop.
	stub.markBad("smart-model", true)
	createEvalModel(t, ts.URL, stub.URL, "fresh-model")
	second := triggerFullSweep(t, ts.URL)
	secondID := int64(second["id"].(float64))
	waitCampaignStatus(t, ts.URL, secondID, store.CampaignStatusDone)

	report2 := getCampaignReport(t, ts.URL, secondID, "")
	baseline2 := reportBaseline(t, report2)
	if baseline2 == nil {
		t.Fatalf("second batch baseline missing: %v", report2)
	}
	if int64(baseline2["campaign_id"].(float64)) != firstID {
		t.Errorf("baseline campaign = %v, want batch %d", baseline2["campaign_id"], firstID)
	}
	if baseline2["comparable"] != true {
		t.Errorf("baseline comparable = %v, want true (same suite versions)", baseline2)
	}
	rows2 := reportRows(t, report2)
	smart2 := rowByModel(t, rows2, "smart-model")
	total2 := totalOf(t, smart2)
	delta, ok := smart2["total_delta"].(float64)
	if !ok {
		t.Fatalf("smart total_delta = %v, want a number", smart2["total_delta"])
	}
	if want := total2 - total1; math.Abs(delta-want) > 1e-9 {
		t.Errorf("smart total_delta = %v, want %v (total %v vs baseline %v)", delta, want, total2, total1)
	}
	if delta >= 0 {
		t.Errorf("degraded smart model should show a negative delta, got %v", delta)
	}
	fresh := rowByModel(t, rows2, "fresh-model")
	if d := fresh["total_delta"]; d != nil {
		t.Errorf("fresh model total_delta = %v, want null (no baseline score)", d)
	}

	// Batch 3: a case edit bumps the basic suite version. Scores stop being
	// comparable, the baseline carries the suite_changed marker, and every
	// delta disappears.
	stub.markBad("smart-model", false)
	patchCase(t, ts.URL, 1, map[string]interface{}{"prompt": "只回复 pong，别的什么都不要说"})
	third := triggerFullSweep(t, ts.URL)
	thirdID := int64(third["id"].(float64))
	waitCampaignStatus(t, ts.URL, thirdID, store.CampaignStatusDone)

	report3 := getCampaignReport(t, ts.URL, thirdID, "")
	baseline3 := reportBaseline(t, report3)
	if baseline3 == nil {
		t.Fatalf("third batch baseline missing: %v", report3)
	}
	if int64(baseline3["campaign_id"].(float64)) != secondID {
		t.Errorf("baseline campaign = %v, want batch %d", baseline3["campaign_id"], secondID)
	}
	if baseline3["comparable"] != false {
		t.Errorf("cross-version baseline comparable = %v, want false", baseline3)
	}
	if baseline3["reason"] != "suite_changed" {
		t.Errorf("baseline reason = %v, want suite_changed", baseline3["reason"])
	}
	for _, row := range reportRows(t, report3) {
		if d := row["total_delta"]; d != nil {
			t.Errorf("cross-version row %v total_delta = %v, want null", row["model_id"], d)
		}
	}
}

// TestCampaignReportDeltaSuiteMissingBaseline pins the conservative
// convention documented on store.PreviousDoneCampaign: a manual
// single-suite campaign between two full sweeps becomes the baseline, and
// the missing suites mark it incomparable (suite_missing) rather than
// skipping back to the last full sweep.
func TestCampaignReportDeltaSuiteMissingBaseline(t *testing.T) {
	ts, stub, _ := setupEvalEnv(t)
	modelID := createEvalModel(t, ts.URL, stub.URL, "smart-model")

	full := triggerFullSweep(t, ts.URL)
	waitCampaignStatus(t, ts.URL, int64(full["id"].(float64)), store.CampaignStatusDone)

	// A manual single-suite campaign lands between the two sweeps.
	basicID := suiteIDByKey(t, ts.URL, "basic")
	runID := triggerEval(t, ts.URL, basicID, modelID)
	run := waitEvalDone(t, ts.URL, runID)
	waitCampaignStatus(t, ts.URL, int64(run["campaign_id"].(float64)), store.CampaignStatusDone)

	next := triggerFullSweep(t, ts.URL)
	nextID := int64(next["id"].(float64))
	waitCampaignStatus(t, ts.URL, nextID, store.CampaignStatusDone)

	report := getCampaignReport(t, ts.URL, nextID, "")
	baseline := reportBaseline(t, report)
	if baseline == nil {
		t.Fatalf("baseline missing: %v", report)
	}
	if baseline["comparable"] != false {
		t.Errorf("single-suite baseline comparable = %v, want false", baseline)
	}
	if baseline["reason"] != "suite_missing" {
		t.Errorf("baseline reason = %v, want suite_missing", baseline["reason"])
	}
	for _, row := range reportRows(t, report) {
		if d := row["total_delta"]; d != nil {
			t.Errorf("row %v total_delta = %v, want null (incomparable baseline)", row["model_id"], d)
		}
	}
}
