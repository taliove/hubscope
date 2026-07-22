package server_test

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"testing"

	"github.com/taliove2009/hubscope/internal/store"
)

// getCampaignReport fetches GET /api/campaigns/{id}/report with an optional
// raw query string and asserts HTTP 200.
func getCampaignReport(t *testing.T, base string, id int64, query string) map[string]interface{} {
	t.Helper()
	url := fmt.Sprintf("%s/api/campaigns/%d/report", base, id)
	if query != "" {
		url += "?" + query
	}
	resp := doGet(t, url)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("GET /api/campaigns/%d/report: expected 200, got %d: %s", id, resp.StatusCode, b)
	}
	var env envelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode report: %v", err)
	}
	var report map[string]interface{}
	if err := json.Unmarshal(env.Data, &report); err != nil {
		t.Fatalf("unmarshal report: %v", err)
	}
	return report
}

// reportRows extracts the leaderboard rows of a report payload.
func reportRows(t *testing.T, report map[string]interface{}) []map[string]interface{} {
	t.Helper()
	raw, ok := report["rows"].([]interface{})
	if !ok {
		t.Fatalf("report rows missing or wrong type: %v", report)
	}
	rows := make([]map[string]interface{}, 0, len(raw))
	for _, r := range raw {
		rows = append(rows, r.(map[string]interface{}))
	}
	return rows
}

// reportWeights extracts the effective-weights echo of a report payload.
func reportWeights(t *testing.T, report map[string]interface{}) map[string]float64 {
	t.Helper()
	raw, ok := report["weights"].(map[string]interface{})
	if !ok {
		t.Fatalf("report weights missing or wrong type: %v", report)
	}
	weights := make(map[string]float64, len(raw))
	for k, v := range raw {
		weights[k] = v.(float64)
	}
	return weights
}

// expectedTotal mirrors the report's total formula (ADR 0005): the weighted
// mean of the row's non-null suite scores (0-100 scale); ok=false when the
// model scored nothing in any suite.
func expectedTotal(t *testing.T, row map[string]interface{}, weights map[string]float64) (float64, bool) {
	t.Helper()
	scores, ok := row["suite_scores"].(map[string]interface{})
	if !ok {
		t.Fatalf("row suite_scores missing or wrong type: %v", row)
	}
	var sum, wsum float64
	for key, raw := range scores {
		if raw == nil {
			continue
		}
		w := weights[key]
		if w <= 0 {
			t.Fatalf("effective weight for %q must be positive, got %v", key, w)
		}
		sum += w * raw.(float64)
		wsum += w
	}
	if wsum == 0 {
		return 0, false
	}
	return sum / wsum, true
}

// assertRowTotals checks every row's total_score against the weighted mean of
// its own suite scores under the report's effective weights.
func assertRowTotals(t *testing.T, report map[string]interface{}) {
	t.Helper()
	weights := reportWeights(t, report)
	for _, row := range reportRows(t, report) {
		want, ok := expectedTotal(t, row, weights)
		got := row["total_score"]
		if !ok {
			if got != nil {
				t.Errorf("model %v: total_score = %v, want null (no scored suite)", row["model_id"], got)
			}
			continue
		}
		gotF, isNum := got.(float64)
		if !isNum {
			t.Errorf("model %v: total_score = %v, want %v", row["model_id"], got, want)
			continue
		}
		if math.Abs(gotF-want) > 1e-9 {
			t.Errorf("model %v: total_score = %v, want weighted mean %v (weights %v)",
				row["model_id"], gotF, want, weights)
		}
	}
}

// TestCampaignReportWeightingAndSorting runs a full sweep over a smart, a
// dumb and a broken model, then asserts the leaderboard contract: equal
// weights by default, custom weights via settings, descending order with
// unscored models last, family filtering, and suite-column sorting.
func TestCampaignReportWeightingAndSorting(t *testing.T) {
	ts, stub, _ := setupEvalEnv(t)
	smartID := createEvalModel(t, ts.URL, stub.URL, "smart-model")
	createEvalModel(t, ts.URL, stub.URL, "dumb-model")
	createEvalModel(t, ts.URL, stub.URL, "broken-model")
	stub.markBroken("broken-model", true)

	campaign := triggerFullSweep(t, ts.URL)
	campaignID := int64(campaign["id"].(float64))
	waitCampaignStatus(t, ts.URL, campaignID, store.CampaignStatusDone)

	// Default report: equal weights, ranked by total descending.
	report := getCampaignReport(t, ts.URL, campaignID, "")
	if report["status"] != store.CampaignStatusDone {
		t.Fatalf("report campaign status = %v, want done", report["status"])
	}
	suites, ok := report["suites"].([]interface{})
	if !ok || len(suites) != suiteCount(t, ts.URL) {
		t.Fatalf("report suites = %v, want one entry per suite", report["suites"])
	}
	weights := reportWeights(t, report)
	for key, w := range weights {
		if w != 1 {
			t.Errorf("default weight for %q = %v, want 1 (equal weighting)", key, w)
		}
	}

	rows := reportRows(t, report)
	if len(rows) != 3 {
		t.Fatalf("expected 3 leaderboard rows, got %v", rows)
	}
	if rows[0]["model_id"] != "smart-model" {
		t.Errorf("rank 1 = %v, want smart-model", rows[0]["model_id"])
	}
	if rows[1]["model_id"] != "dumb-model" {
		t.Errorf("rank 2 = %v, want dumb-model", rows[1]["model_id"])
	}
	if rows[2]["model_id"] != "broken-model" || rows[2]["total_score"] != nil {
		t.Errorf("unscored model must rank last with null total, got %v", rows[2])
	}
	if got := rows[0]["total_score"].(float64); got <= 0 || got > 100 {
		t.Errorf("smart total on 0-100 scale, got %v", got)
	}
	assertRowTotals(t, report)

	// Custom weights via settings: cap_instruction counts triple.
	putResp := doPut(t, ts.URL+"/api/settings", map[string]interface{}{
		"suite_weights": map[string]interface{}{"cap_instruction": 3},
	})
	putResp.Body.Close()
	if putResp.StatusCode != http.StatusOK {
		t.Fatalf("PUT suite_weights: expected 200, got %d", putResp.StatusCode)
	}
	weighted := getCampaignReport(t, ts.URL, campaignID, "")
	if got := reportWeights(t, weighted)["cap_instruction"]; got != 3 {
		t.Errorf("effective weight for cap_instruction = %v, want 3", got)
	}
	assertRowTotals(t, weighted)

	// Sorting by a suite column keeps the same descending contract.
	byInstruction := getCampaignReport(t, ts.URL, campaignID, "sort=cap_instruction")
	instructionRows := reportRows(t, byInstruction)
	if instructionRows[0]["model_id"] != "smart-model" || instructionRows[2]["model_id"] != "broken-model" {
		t.Errorf("sort=cap_instruction order = [%v %v %v], want smart first, broken last",
			instructionRows[0]["model_id"], instructionRows[1]["model_id"], instructionRows[2]["model_id"])
	}
	instructionScores, _ := instructionRows[0]["suite_scores"].(map[string]interface{})
	if instructionScores["cap_instruction"] != 100.0 {
		t.Errorf("smart cap_instruction suite score = %v, want 100", instructionScores["cap_instruction"])
	}

	// Unknown sort column is rejected.
	badSort := doGet(t, fmt.Sprintf("%s/api/campaigns/%d/report?sort=nosuch", ts.URL, campaignID))
	badSort.Body.Close()
	if badSort.StatusCode != http.StatusBadRequest {
		t.Errorf("sort=nosuch: expected 400, got %d", badSort.StatusCode)
	}

	// Family filter: tag the smart model with its own family, then filter.
	ruleResp := doPost(t, ts.URL+"/api/classification-rules", map[string]interface{}{
		"dimension": "family",
		"keyword":   "smart",
		"category":  "acme",
		"priority":  10,
	})
	ruleResp.Body.Close()
	if ruleResp.StatusCode != http.StatusCreated {
		t.Fatalf("create family rule: expected 201, got %d", ruleResp.StatusCode)
	}
	filtered := getCampaignReport(t, ts.URL, campaignID, "family=acme")
	acmeRows := reportRows(t, filtered)
	if len(acmeRows) != 1 || acmeRows[0]["model_id"] != "smart-model" {
		t.Errorf("family=acme rows = %v, want only smart-model", acmeRows)
	}
	if acmeRows[0]["family"] != "acme" {
		t.Errorf("smart row family = %v, want acme", acmeRows[0]["family"])
	}
	none := getCampaignReport(t, ts.URL, campaignID, "family=nosuch")
	if len(reportRows(t, none)) != 0 {
		t.Errorf("family=nosuch should yield no rows, got %v", none["rows"])
	}

	// The models endpoint sanity check keeps the family assertion honest:
	// the dumb model must not have been tagged acme.
	if int64(acmeRows[0]["model_db_id"].(float64)) != smartID {
		t.Errorf("filtered row model_db_id = %v, want smart model %d", acmeRows[0]["model_db_id"], smartID)
	}
}

// TestCampaignReportHidesDeletedModels evaluates a manual model plus a
// discovered model, then asserts both deletion semantics of ticket 26 keep
// them off the leaderboard: a retired discovered model and a deleted manual
// model both disappear from the report while it stays readable.
func TestCampaignReportHidesDeletedModels(t *testing.T) {
	ts, stub, _ := setupEvalEnv(t)
	keepID := createEvalModel(t, ts.URL, stub.URL, "keep-model")

	discovery := newDiscoveryStubHub(t, []string{"vanish-model"})
	createHubViaAPI(t, ts.URL, discovery.URL)
	runDiscovery(t, ts.URL)
	models := listModelsViaAPI(t, ts.URL)
	vanish, ok := models["vanish-model"]
	if !ok {
		t.Fatalf("vanish-model not discovered: %v", models)
	}
	vanishID := int64(vanish["id"].(float64))

	basicID := suiteIDByKey(t, ts.URL, "basic")
	runID := triggerEval(t, ts.URL, basicID, keepID, vanishID)
	run := waitEvalDone(t, ts.URL, runID)
	campaignID := int64(run["campaign_id"].(float64))
	waitCampaignStatus(t, ts.URL, campaignID, store.CampaignStatusDone)

	// Baseline: both models rank.
	rows := reportRows(t, getCampaignReport(t, ts.URL, campaignID, ""))
	if len(rows) != 2 {
		t.Fatalf("before deletion: expected 2 rows, got %v", rows)
	}

	// Retire the discovered model: the hub stops listing it (an empty list
	// would be treated as a fetch anomaly and retire nothing), and it
	// vanishes from the leaderboard.
	discovery.setModels([]string{"replacement-model"})
	runDiscovery(t, ts.URL)
	rows = reportRows(t, getCampaignReport(t, ts.URL, campaignID, ""))
	if len(rows) != 1 || rows[0]["model_id"] != "keep-model" {
		t.Errorf("after retire: expected only keep-model, got %v", rows)
	}

	// Delete the manual model: the leaderboard is empty but still 200.
	delResp := doDelete(t, fmt.Sprintf("%s/api/models/%d", ts.URL, keepID))
	delResp.Body.Close()
	if delResp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete keep-model: expected 204, got %d", delResp.StatusCode)
	}
	rows = reportRows(t, getCampaignReport(t, ts.URL, campaignID, ""))
	if len(rows) != 0 {
		t.Errorf("after delete: expected no rows, got %v", rows)
	}
}

// TestCampaignReportSettingsValidation covers the suite_weights setting:
// validation of keys and values, the settings round-trip, and the report's
// 404 for unknown campaigns.
func TestCampaignReportSettingsValidation(t *testing.T) {
	ts, _, _ := setupEvalEnv(t)

	// Default: no weights configured, the settings read yields an empty map.
	getResp := doGet(t, ts.URL+"/api/settings")
	var env envelope
	_ = json.NewDecoder(getResp.Body).Decode(&env)
	getResp.Body.Close()
	var settings map[string]interface{}
	_ = json.Unmarshal(env.Data, &settings)
	if w, ok := settings["suite_weights"].(map[string]interface{}); !ok || len(w) != 0 {
		t.Errorf("default suite_weights = %v, want empty object", settings["suite_weights"])
	}

	// Unknown suite keys and non-positive or absurd weights are rejected.
	for _, body := range []map[string]interface{}{
		{"suite_weights": map[string]interface{}{"nosuch": 1}},
		{"suite_weights": map[string]interface{}{"basic": 0}},
		{"suite_weights": map[string]interface{}{"basic": -2}},
		{"suite_weights": map[string]interface{}{"basic": 1e308}},
	} {
		resp := doPut(t, ts.URL+"/api/settings", body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("PUT %v: expected 400, got %d", body, resp.StatusCode)
		}
	}

	// A valid map round-trips through the settings API.
	putResp := doPut(t, ts.URL+"/api/settings", map[string]interface{}{
		"suite_weights": map[string]interface{}{"basic": 2, "reasoning": 0.5},
	})
	putResp.Body.Close()
	if putResp.StatusCode != http.StatusOK {
		t.Fatalf("PUT valid suite_weights: expected 200, got %d", putResp.StatusCode)
	}
	getResp = doGet(t, ts.URL+"/api/settings")
	_ = json.NewDecoder(getResp.Body).Decode(&env)
	getResp.Body.Close()
	settings = nil
	_ = json.Unmarshal(env.Data, &settings)
	saved, _ := settings["suite_weights"].(map[string]interface{})
	if saved["basic"] != 2.0 || saved["reasoning"] != 0.5 {
		t.Errorf("saved suite_weights = %v, want basic=2 reasoning=0.5", saved)
	}

	// Unknown campaign: report is a 404, like the campaign detail.
	missing := doGet(t, ts.URL+"/api/campaigns/9999/report")
	missing.Body.Close()
	if missing.StatusCode != http.StatusNotFound {
		t.Errorf("report of missing campaign: expected 404, got %d", missing.StatusCode)
	}
}

// TestCampaignReportRunningBatchReturnsNoRows pins the zero-result edge of
// the spec 0004 live board: a running campaign whose only run has not
// recorded any result yet serves an empty row list (runs carry no model
// membership, so untouched models have no cells) — progress comes from the
// campaign counters alone. Once results exist the rows appear (covered by
// TestCampaignReportProgressGrid), and once the batch settles the ranked
// board replaces the live one.
func TestCampaignReportRunningBatchReturnsNoRows(t *testing.T) {
	ts, stub, _ := setupEvalEnv(t)
	modelID := createEvalModel(t, ts.URL, stub.URL, "smart-model")
	stub.blockCalls()
	t.Cleanup(stub.release)

	basicID := suiteIDByKey(t, ts.URL, "basic")
	runID := triggerEval(t, ts.URL, basicID, modelID)
	waitFor(t, "eval call reaching the stub", func() bool {
		return stub.sawModel("smart-model")
	})

	run := getEvalRun(t, ts.URL, runID)
	campaignID := int64(run["campaign_id"].(float64))
	report := getCampaignReport(t, ts.URL, campaignID, "")
	if report["status"] != "running" {
		t.Fatalf("campaign status = %v, want running", report["status"])
	}
	if rows := reportRows(t, report); len(rows) != 0 {
		t.Errorf("running campaign must return no leaderboard rows, got %v", rows)
	}
	progress := campaignProgress(t, report)
	if int(progress["total"].(float64)) != 1 {
		t.Errorf("running campaign progress.total = %v, want 1", progress)
	}

	stub.release()
	waitCampaignStatus(t, ts.URL, campaignID, store.CampaignStatusDone)
	report = getCampaignReport(t, ts.URL, campaignID, "")
	if rows := reportRows(t, report); len(rows) != 1 {
		t.Errorf("done campaign must return the model's row, got %v", rows)
	}
}
