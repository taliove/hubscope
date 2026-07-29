package server_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/taliove/hubscope/internal/store"
)

// setupCostCampaign builds a running campaign with two models whose results
// carry latency/token data: cost-model-a has one result with tokens and one
// without (null tokens count as 0 in sums); cost-model-b's only result has
// no token record at all (its cost-row token fields must come back null).
// Returns the campaign id and the suite used.
func setupCostCampaign(t *testing.T, db *store.DB) (int64, store.Suite) {
	t.Helper()
	hub, err := db.CreateHub("cost-hub", "http://cost.test", "tok-cost-0000")
	if err != nil {
		t.Fatalf("create hub: %v", err)
	}
	modelA, err := db.CreateModel(hub.ID, "cost-model-a", []string{"openai"})
	if err != nil {
		t.Fatalf("create model a: %v", err)
	}
	modelB, err := db.CreateModel(hub.ID, "cost-model-b", []string{"openai"})
	if err != nil {
		t.Fatalf("create model b: %v", err)
	}
	suites, err := db.ListSuites()
	if err != nil || len(suites) == 0 {
		t.Fatalf("list suites: %v (n=%d)", err, len(suites))
	}
	suite := suites[0]
	cases, err := db.ListCases(suite.ID)
	if err != nil || len(cases) < 2 {
		t.Fatalf("list cases: %v (n=%d)", err, len(cases))
	}

	campaign, err := db.CreateCampaign("manual", []int64{modelA.ID, modelB.ID}, time.Now().UTC())
	if err != nil {
		t.Fatalf("create campaign: %v", err)
	}
	run, err := db.CreateEvalRun(campaign.ID, suite.ID, "manual", "judge-x")
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	one, half := 1.0, 0.5
	in, out := 10, 20
	mk := func(model *store.Model, caseID int64, score *float64, latency int, tin, tout *int) {
		t.Helper()
		if _, err := db.CreateEvalResult(store.EvalResult{
			EvalRunID: run.ID, ModelDBID: model.ID, ModelID: model.ModelID, CaseID: caseID,
			Score: score, LatencyMs: latency, InputTokens: tin, OutputTokens: tout,
		}); err != nil {
			t.Fatalf("create result (%s): %v", model.ModelID, err)
		}
	}
	mk(modelA, cases[0].ID, &one, 120, &in, &out)
	mk(modelA, cases[1].ID, &half, 240, nil, nil) // null tokens count as 0
	mk(modelB, cases[0].ID, &one, 100, nil, nil)  // no token record at all
	if err := db.FinishEvalRun(run.ID, "done", time.Now().UTC()); err != nil {
		t.Fatalf("finish run: %v", err)
	}
	return campaign.ID, suite
}

// TestCampaignReportCostFields pins the GH #42 console-side cost contract:
// the session report carries the batch cost totals (Σ latency / tokens over
// every result, null tokens counted as 0), the per-(model, suite-run) cost
// rows (token fields null only when the model recorded no token at all in
// that run), and the per-cell cost sums (same caliber) on both the live and
// the settled board.
func TestCampaignReportCostFields(t *testing.T) {
	db := openTempDB(t)
	ts := newTestAPIServer(t, db)
	campaignID, suite := setupCostCampaign(t, db)

	assertCostPayload := func(report map[string]interface{}, phase string) {
		t.Helper()
		cost, ok := report["cost"].(map[string]interface{})
		if !ok {
			t.Fatalf("%s: report.cost missing or wrong type: %v", phase, report)
		}
		if cost["latency_ms"] != 460.0 {
			t.Errorf("%s: cost.latency_ms = %v, want 460 (120+240+100)", phase, cost["latency_ms"])
		}
		if cost["input_tokens"] != 10.0 || cost["output_tokens"] != 20.0 {
			t.Errorf("%s: cost tokens = %v/%v, want 10/20 (nulls counted as 0)", phase, cost["input_tokens"], cost["output_tokens"])
		}

		rawRows, ok := report["cost_rows"].([]interface{})
		if !ok || len(rawRows) != 2 {
			t.Fatalf("%s: cost_rows = %v, want 2 rows (one per model x run)", phase, report["cost_rows"])
		}
		byModel := map[string]map[string]interface{}{}
		for _, r := range rawRows {
			row := r.(map[string]interface{})
			byModel[row["model_id"].(string)] = row
		}
		rowA := byModel["cost-model-a"]
		if rowA == nil || rowA["suite_key"] != suite.Key || rowA["suite_name"] != suite.Name {
			t.Fatalf("%s: cost row a = %v, want suite %s/%s", phase, rowA, suite.Key, suite.Name)
		}
		if rowA["status"] != "done" {
			t.Errorf("%s: cost row a status = %v, want done", phase, rowA["status"])
		}
		if rowA["latency_ms"] != 360.0 || rowA["input_tokens"] != 10.0 || rowA["output_tokens"] != 20.0 {
			t.Errorf("%s: cost row a = %v, want latency 360 / tokens 10/20", phase, rowA)
		}
		rowB := byModel["cost-model-b"]
		if rowB == nil {
			t.Fatalf("%s: cost row for cost-model-b missing: %v", phase, rawRows)
		}
		if rowB["latency_ms"] != 100.0 {
			t.Errorf("%s: cost row b latency = %v, want 100", phase, rowB["latency_ms"])
		}
		if v, present := rowB["input_tokens"]; !present || v != nil {
			t.Errorf("%s: cost row b input_tokens = %v (present=%v), want explicit null (no token record)", phase, v, present)
		}
		if v, present := rowB["output_tokens"]; !present || v != nil {
			t.Errorf("%s: cost row b output_tokens = %v (present=%v), want explicit null", phase, v, present)
		}

		// Cell-level sums ride the progress cells with the same caliber.
		for _, row := range reportRows(t, report) {
			for _, c := range row["cells"].([]interface{}) {
				cell := c.(map[string]interface{})
				if cell["suite_key"] != suite.Key {
					continue
				}
				switch row["model_id"] {
				case "cost-model-a":
					if cell["latency_ms"] != 360.0 || cell["input_tokens"] != 10.0 || cell["output_tokens"] != 20.0 {
						t.Errorf("%s: cell a = %v, want latency 360 / tokens 10/20", phase, cell)
					}
				case "cost-model-b":
					if cell["latency_ms"] != 100.0 || cell["input_tokens"] != 0.0 || cell["output_tokens"] != 0.0 {
						t.Errorf("%s: cell b = %v, want latency 100 / tokens 0/0 (nulls as 0 in cell sums)", phase, cell)
					}
				}
			}
		}
	}

	// Live board (campaign still running): cost fields present.
	live := getCampaignReport(t, ts.URL, campaignID, "")
	if live["status"] != "running" {
		t.Fatalf("campaign status = %v, want running", live["status"])
	}
	assertCostPayload(live, "live")

	// Settled board: same cost payload.
	if err := db.SettleCampaign(campaignID, time.Now().UTC()); err != nil {
		t.Fatalf("settle campaign: %v", err)
	}
	settled := getCampaignReport(t, ts.URL, campaignID, "")
	if settled["status"] != "done" {
		t.Fatalf("settled status = %v, want done", settled["status"])
	}
	assertCostPayload(settled, "settled")
}

// TestCampaignReportCostConsoleOnly pins the GH #42 boundary (main ruling
// 2026-07-29): cost is operational data and never crosses the session
// boundary — the token-gated shared report and the anonymous public eval
// board carry no cost totals, no cost rows and no per-cell cost fields, on
// neither the settled board nor the in-flight progress board.
func TestCampaignReportCostConsoleOnly(t *testing.T) {
	db := openTempDB(t)
	ts := newTestAPIServer(t, db)
	campaignID, _ := setupCostCampaign(t, db)

	link := createShareLink(t, ts.URL, campaignID)
	token := link["token"].(string)

	assertNoCost := func(report map[string]interface{}, phase string) {
		t.Helper()
		if _, ok := report["cost"]; ok {
			t.Errorf("%s: cost key leaked across the session boundary", phase)
		}
		if _, ok := report["cost_rows"]; ok {
			t.Errorf("%s: cost_rows key leaked across the session boundary", phase)
		}
		for _, row := range reportRows(t, report) {
			for _, c := range row["cells"].([]interface{}) {
				cell := c.(map[string]interface{})
				for _, key := range []string{"latency_ms", "input_tokens", "output_tokens"} {
					if _, ok := cell[key]; ok {
						t.Errorf("%s: cell %s/%s leaked %q across the session boundary", phase, row["model_id"], cell["suite_key"], key)
					}
				}
			}
		}
	}

	// Shared report of the in-flight batch: stripped progress board, no cost.
	shared := getSharedReport(t, ts.URL, token)
	assertNoCost(shared, "shared running")

	// Settle, then re-check both public surfaces on the full board.
	if err := db.SettleCampaign(campaignID, time.Now().UTC()); err != nil {
		t.Fatalf("settle campaign: %v", err)
	}
	assertNoCost(getSharedReport(t, ts.URL, token), "shared settled")

	resp := plainGet(t, ts.URL+"/api/public/eval/board")
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("public board: expected 200, got %d", resp.StatusCode)
	}
	var env envelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode public board: %v", err)
	}
	var board map[string]interface{}
	if err := json.Unmarshal(env.Data, &board); err != nil {
		t.Fatalf("unmarshal public board: %v", err)
	}
	report, ok := board["report"].(map[string]interface{})
	if !ok {
		t.Fatalf("public board report missing: %v", board)
	}
	assertNoCost(report, "public board")
}
