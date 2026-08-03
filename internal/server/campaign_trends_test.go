package server_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/taliove/hubscope/internal/store"
)

// getCampaignTrends fetches GET /api/campaigns/{id}/trends with the given raw
// query string and asserts HTTP 200.
func getCampaignTrends(t *testing.T, base string, id int64, query string) map[string]interface{} {
	t.Helper()
	url := fmt.Sprintf("%s/api/campaigns/%d/trends", base, id)
	if query != "" {
		url += "?" + query
	}
	resp := doGet(t, url)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("GET /api/campaigns/%d/trends: expected 200, got %d: %s", id, resp.StatusCode, b)
	}
	var env envelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode trends: %v", err)
	}
	var trends map[string]interface{}
	if err := json.Unmarshal(env.Data, &trends); err != nil {
		t.Fatalf("unmarshal trends: %v", err)
	}
	return trends
}

// trendModel extracts the model identity block of a trends payload.
func trendModel(t *testing.T, trends map[string]interface{}) map[string]interface{} {
	t.Helper()
	model, ok := trends["model"].(map[string]interface{})
	if !ok {
		t.Fatalf("trends model missing or wrong type: %v", trends)
	}
	return model
}

// trendSuites extracts the per-suite trend series of a trends payload.
func trendSuites(t *testing.T, trends map[string]interface{}) []map[string]interface{} {
	t.Helper()
	raw, ok := trends["suites"].([]interface{})
	if !ok {
		t.Fatalf("trends suites missing or wrong type: %v", trends)
	}
	suites := make([]map[string]interface{}, 0, len(raw))
	for _, s := range raw {
		suites = append(suites, s.(map[string]interface{}))
	}
	return suites
}

// trendProbeBuckets extracts the probe-side hourly buckets of a trends payload.
func trendProbeBuckets(t *testing.T, trends map[string]interface{}) []map[string]interface{} {
	t.Helper()
	raw, ok := trends["probe"].([]interface{})
	if !ok {
		t.Fatalf("trends probe missing or wrong type: %v", trends)
	}
	buckets := make([]map[string]interface{}, 0, len(raw))
	for _, b := range raw {
		buckets = append(buckets, b.(map[string]interface{}))
	}
	return buckets
}

// firstEndpointID returns the first endpoint id of the given model via
// GET /api/models.
func firstEndpointID(t *testing.T, base string, modelDBID int64) int64 {
	t.Helper()
	resp := doGet(t, base+"/api/models")
	defer resp.Body.Close()
	var env envelope
	_ = json.NewDecoder(resp.Body).Decode(&env)
	var models []map[string]interface{}
	_ = json.Unmarshal(env.Data, &models)
	for _, m := range models {
		if int64(m["id"].(float64)) != modelDBID {
			continue
		}
		endpoints := m["endpoints"].([]interface{})
		if len(endpoints) == 0 {
			t.Fatalf("model %d has no endpoints", modelDBID)
		}
		return int64(endpoints[0].(map[string]interface{})["id"].(float64))
	}
	t.Fatalf("model %d not found in list", modelDBID)
	return 0
}

// probeEndpointOnce runs one probe round on the endpoint and asserts every
// record matches the expected ok-ness.
func probeEndpointOnce(t *testing.T, base string, endpointID int64, wantOK bool) {
	t.Helper()
	resp := doPost(t, fmt.Sprintf("%s/api/endpoints/%d/probe", base, endpointID), nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("POST probe: expected 200, got %d: %s", resp.StatusCode, b)
	}
	var env envelope
	_ = json.NewDecoder(resp.Body).Decode(&env)
	var result map[string]interface{}
	_ = json.Unmarshal(env.Data, &result)
	results := result["results"].([]interface{})
	if len(results) != 2 {
		t.Fatalf("probe round returned %d records, want 2", len(results))
	}
	for _, r := range results {
		if got := r.(map[string]interface{})["ok"].(bool); got != wantOK {
			t.Fatalf("probe record ok = %v, want %v", got, wantOK)
		}
	}
}

// TestCampaignTrends covers the ticket-32 trend contract: per-(model, suite)
// points ordered by campaign with the suite version each campaign scored
// against, a break marker where the version changes, an unjudged campaign
// staying a visible null-score point, the probe-side rollup on the same
// timeline, and a deleted model keeping its trend with the deleted flag.
func TestCampaignTrends(t *testing.T) {
	ts, stub, db := setupEvalEnv(t)
	modelID := createEvalModel(t, ts.URL, stub.URL, "smart-model")
	instructionID := suiteIDByKey(t, ts.URL, "gsm8k")
	// One custom exact-rule case (seeded cases retired): the smart model
	// scores 100, and the case is the version-bump target below.
	retireSuiteCases(t, db, instructionID)
	createRuleCase(t, ts.URL, instructionID, "TREND-A:请作答", "好的", nil)
	var trendCaseID int64
	for _, c := range suiteByKey(t, ts.URL, "gsm8k")["cases"].([]interface{}) {
		cm := c.(map[string]interface{})
		if cm["prompt"] == "TREND-A:请作答" {
			trendCaseID = int64(cm["id"].(float64))
		}
	}
	if trendCaseID == 0 {
		t.Fatal("custom trend case not found")
	}

	// Campaign 1 on suite v101: the smart model scores 100.
	run1 := triggerEval(t, ts.URL, instructionID, modelID)
	waitEvalDone(t, ts.URL, run1)
	c1 := int64(getEvalRun(t, ts.URL, run1)["campaign_id"].(float64))
	waitCampaignStatus(t, ts.URL, c1, store.CampaignStatusDone)

	// One healthy probe round on the model's first endpoint.
	endpointID := firstEndpointID(t, ts.URL, modelID)
	probeEndpointOnce(t, ts.URL, endpointID, true)

	// Editing the case bumps the question bank one version.
	patchCase(t, ts.URL, trendCaseID, map[string]interface{}{"prompt": "只回复 pong，别的什么都不要说"})

	// Campaign 2 on v2 with the model failing at case time (GH #174: it
	// passes the probe gate, then every case call 503s): answers fail,
	// scores stay unjudged (null) — the trend point must remain visible,
	// not vanish.
	stub.markCaseBroken("smart-model", true)
	run2 := triggerEval(t, ts.URL, instructionID, modelID)
	waitEvalDone(t, ts.URL, run2)
	c2 := int64(getEvalRun(t, ts.URL, run2)["campaign_id"].(float64))
	waitCampaignStatus(t, ts.URL, c2, store.CampaignStatusDone)

	// One failing probe round while the model fails cases (the monitoring
	// prober's prompt is not the gate's, so the case-broken model fails
	// monitoring probes too).
	probeEndpointOnce(t, ts.URL, endpointID, false)
	stub.markCaseBroken("smart-model", false)

	trends := getCampaignTrends(t, ts.URL, c2, fmt.Sprintf("model=%d", modelID))

	model := trendModel(t, trends)
	if model["model_id"] != "smart-model" {
		t.Errorf("trend model_id = %v, want smart-model", model["model_id"])
	}
	if model["deleted"] != false {
		t.Errorf("live model deleted = %v, want false", model["deleted"])
	}

	suites := trendSuites(t, trends)
	if len(suites) != 1 || suites[0]["key"] != "gsm8k" {
		t.Fatalf("trend suites = %v, want exactly the reasoning suite", suites)
	}
	points, ok := suites[0]["points"].([]interface{})
	if !ok || len(points) != 2 {
		t.Fatalf("instruction trend points = %v, want 2 (one per campaign)", suites[0]["points"])
	}
	p0 := points[0].(map[string]interface{})
	p1 := points[1].(map[string]interface{})

	// Points are ordered by campaign, each carrying its suite version.
	if int64(p0["campaign_id"].(float64)) != c1 || int64(p1["campaign_id"].(float64)) != c2 {
		t.Errorf("point campaign order = [%v %v], want [%d %d]", p0["campaign_id"], p1["campaign_id"], c1, c2)
	}
	v0 := p0["suite_version"].(float64)
	if p1["suite_version"] != v0+1 {
		t.Errorf("point suite versions = [%v %v], want [v v+1]", p0["suite_version"], p1["suite_version"])
	}
	if p0["score"] != 100.0 {
		t.Errorf("campaign 1 score = %v, want 100", p0["score"])
	}
	// The broken campaign keeps a visible null point, not a fake zero.
	if p1["score"] != nil {
		t.Errorf("unjudged campaign score = %v, want null", p1["score"])
	}
	// Break marker: only the point where the version changed is flagged.
	if p0["version_changed"] != false || p1["version_changed"] != true {
		t.Errorf("version_changed = [%v %v], want [false true]", p0["version_changed"], p1["version_changed"])
	}

	// Probe side: hourly buckets over the same timeline. Only the broken
	// round failed (2 records), creation trials and the healthy round passed.
	buckets := trendProbeBuckets(t, trends)
	if len(buckets) == 0 {
		t.Fatal("probe buckets empty, want the model's probe history")
	}
	totalN, failureN := 0, 0
	for _, b := range buckets {
		totalN += int(b["total"].(float64))
		failureN += int(b["failures"].(float64))
		if b["p50_ms"] == nil {
			t.Errorf("bucket %v missing p50", b["bucket_start"])
		}
	}
	if failureN != 2 {
		t.Errorf("probe failures = %d, want exactly 2 (the broken round)", failureN)
	}
	if totalN < 4 {
		t.Errorf("probe total = %d, want at least the 4 manual-round records", totalN)
	}

	// Scoping: the older campaign's trend stops at itself.
	scoped := getCampaignTrends(t, ts.URL, c1, fmt.Sprintf("model=%d", modelID))
	scopedSuites := trendSuites(t, scoped)
	if len(scopedSuites) != 1 {
		t.Fatalf("scoped trend suites = %v, want 1", scopedSuites)
	}
	if got := scopedSuites[0]["points"].([]interface{}); len(got) != 1 {
		t.Errorf("campaign %d trend has %d points, want only its own", c1, len(got))
	}

	// After deletion the trend stays readable, flagged deleted, and the probe
	// side empties out (the endpoints are gone with the model).
	delResp := doDelete(t, fmt.Sprintf("%s/api/models/%d", ts.URL, modelID))
	delResp.Body.Close()
	if delResp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete model: expected 204, got %d", delResp.StatusCode)
	}
	deleted := getCampaignTrends(t, ts.URL, c2, fmt.Sprintf("model=%d", modelID))
	deletedModel := trendModel(t, deleted)
	if deletedModel["deleted"] != true {
		t.Errorf("deleted model flag = %v, want true", deletedModel["deleted"])
	}
	if deletedModel["model_id"] != "smart-model" {
		t.Errorf("deleted model_id = %v, want smart-model (denormalized)", deletedModel["model_id"])
	}
	deletedSuites := trendSuites(t, deleted)
	if len(deletedSuites) != 1 || len(deletedSuites[0]["points"].([]interface{})) != 2 {
		t.Errorf("deleted model trend points = %v, want the same 2 points", deletedSuites)
	}
	if buckets := trendProbeBuckets(t, deleted); len(buckets) != 0 {
		t.Errorf("deleted model probe buckets = %v, want empty", buckets)
	}
}

// TestCampaignTrendsValidation covers the error contract: unknown campaign,
// missing/invalid model parameter, and a model with no history at all.
func TestCampaignTrendsValidation(t *testing.T) {
	ts, _, _ := setupEvalEnv(t)

	missing := doGet(t, ts.URL+"/api/campaigns/9999/trends?model=1")
	missing.Body.Close()
	if missing.StatusCode != http.StatusNotFound {
		t.Errorf("trends of missing campaign: expected 404, got %d", missing.StatusCode)
	}

	// A real campaign is needed for the parameter checks: run one quick eval.
	ts2, stub, _ := setupEvalEnv(t)
	modelID := createEvalModel(t, ts2.URL, stub.URL, "smart-model")
	instructionID := suiteIDByKey(t, ts2.URL, "gsm8k")
	runID := triggerEval(t, ts2.URL, instructionID, modelID)
	waitEvalDone(t, ts2.URL, runID)
	campaignID := int64(getEvalRun(t, ts2.URL, runID)["campaign_id"].(float64))
	waitCampaignStatus(t, ts2.URL, campaignID, store.CampaignStatusDone)

	for _, query := range []string{"", "model=abc", "model=-1", "model=0"} {
		resp := doGet(t, fmt.Sprintf("%s/api/campaigns/%d/trends?%s", ts2.URL, campaignID, query))
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("trends query %q: expected 400, got %d", query, resp.StatusCode)
		}
	}

	// A model id that never existed and has no eval history is a 404.
	unknown := doGet(t, fmt.Sprintf("%s/api/campaigns/%d/trends?model=9999", ts2.URL, campaignID))
	unknown.Body.Close()
	if unknown.StatusCode != http.StatusNotFound {
		t.Errorf("trends of unknown model: expected 404, got %d", unknown.StatusCode)
	}
}
