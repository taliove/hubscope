package server_test

import (
	"testing"

	"github.com/taliove/hubscope/internal/store"
)

// reportCells extracts the per-suite progress cells of one leaderboard row.
func reportCells(t *testing.T, row map[string]interface{}) []map[string]interface{} {
	t.Helper()
	raw, ok := row["cells"].([]interface{})
	if !ok {
		t.Fatalf("row cells missing or wrong type: %v", row)
	}
	cells := make([]map[string]interface{}, 0, len(raw))
	for _, c := range raw {
		cells = append(cells, c.(map[string]interface{}))
	}
	return cells
}

// cellBySuiteKey indexes a row's cells by suite key for direct lookup.
func cellBySuiteKey(t *testing.T, row map[string]interface{}) map[string]map[string]interface{} {
	t.Helper()
	out := map[string]map[string]interface{}{}
	for _, cell := range reportCells(t, row) {
		key, ok := cell["suite_key"].(string)
		if !ok {
			t.Fatalf("cell suite_key missing or wrong type: %v", cell)
		}
		out[key] = cell
	}
	return out
}

// assertCell checks one progress cell's status and judged/expected coverage.
func assertCell(t *testing.T, row map[string]interface{}, suiteKey, status string, judged, expected int) {
	t.Helper()
	cell := cellBySuiteKey(t, row)[suiteKey]
	if cell == nil {
		t.Fatalf("model %v has no cell for suite %q: %v", row["model_id"], suiteKey, row["cells"])
	}
	if cell["status"] != status {
		t.Errorf("model %v suite %s cell status = %v, want %s", row["model_id"], suiteKey, cell["status"], status)
	}
	if got := int(cell["judged_cases"].(float64)); got != judged {
		t.Errorf("model %v suite %s judged_cases = %d, want %d", row["model_id"], suiteKey, got, judged)
	}
	if got := int(cell["expected_cases"].(float64)); got != expected {
		t.Errorf("model %v suite %s expected_cases = %d, want %d", row["model_id"], suiteKey, got, expected)
	}
}

// TestCampaignReportProgressGrid pins the ticket 52 report contract for an
// unfinished campaign: the report exposes per-(model, suite) progress cells
// (status + judged/expected coverage), the half-scored board lists every
// model with recorded results ranked by the half-scored total descending
// (GH #40 — ties break by model id, null totals sink), unscored suites stay
// out of the totals, and no baseline/delta information leaks before the
// batch settles. After the sweep completes, the same endpoint serves the
// full ranked board with every cell done.
//
// Scenario: a full sweep over three models where alpha's answer calls all
// fail (unjudged results) and the judge model is frozen after three judge
// calls. Judge cases exist only in cap_language (the last suite in the
// rotation, 4 judge cases at 3 samples each), so the freeze catches the
// campaign with the four rule-only suites fully done and cap_language
// mid-flight: alpha's cap_language results are complete (all broken, no
// judge calls needed), beta has its six rule cases plus the first judge case
// (3 samples) judged and is blocked on the fourth judge call inside the
// second judge case, and gamma has no cap_language results yet.
func TestCampaignReportProgressGrid(t *testing.T) {
	// Async eval: observes the running report with cap_language frozen
	// mid-flight (judge gate after 3 calls); drained by releaseModel +
	// waitCampaignStatus(done).
	ts, stub, _ := setupAsyncEvalEnv(t)
	createEvalModel(t, ts.URL, stub.URL, "alpha-model")
	createEvalModel(t, ts.URL, stub.URL, "beta-model")
	createEvalModel(t, ts.URL, stub.URL, "gamma-model")
	stub.markBroken("alpha-model", true)
	// Serial cell order (GH #26 pool at 1): the scenario pins gamma's
	// cap_language cell as not-yet-started at the freeze point, which only a
	// deterministic suite→model order guarantees.
	setEvalConcurrency(t, ts.URL, 1)
	stub.resetCalls()
	stub.blockModelAfter(store.DefaultJudgeModel, 3)
	t.Cleanup(func() { stub.releaseModel(store.DefaultJudgeModel) })

	campaign := triggerFullSweep(t, ts.URL)
	campaignID := int64(campaign["id"].(float64))
	// The fourth judge call being recorded proves the freeze point: beta is
	// blocked mid-cap_language, everything before it has settled
	// deterministically.
	waitFor(t, "fourth judge call reaching the stub", func() bool {
		return stub.callTotal(store.DefaultJudgeModel) >= 4
	})

	report := getCampaignReport(t, ts.URL, campaignID, "")
	if report["status"] != store.CampaignStatusRunning {
		t.Fatalf("campaign status = %v, want running", report["status"])
	}

	suites, ok := report["suites"].([]interface{})
	if !ok || len(suites) != suiteCount(t, ts.URL) {
		t.Fatalf("running report suites = %v, want one per suite in the rotation", report["suites"])
	}
	progress := campaignProgress(t, report)
	if got := int(progress["done"].(float64)); got != 4 {
		t.Errorf("progress.done = %d, want 4 (rule-only suites settled)", got)
	}
	if got := int(progress["running"].(float64)); got != 1 {
		t.Errorf("progress.running = %d, want 1 (cap_language mid-flight)", got)
	}

	// The live board ranks by the half-scored total descending (GH #40):
	// beta and gamma both total 100 (their four done rule suites) and tie
	// into model-id order; alpha judged nothing anywhere and sinks to the
	// bottom with a null total.
	rows := reportRows(t, report)
	if len(rows) != 3 {
		t.Fatalf("running report rows = %v, want beta, gamma, alpha", rows)
	}
	for i, want := range []string{"beta-model", "gamma-model", "alpha-model"} {
		if rows[i]["model_id"] != want {
			t.Errorf("live board row %d = %v, want %s (partial total desc, null sunk)", i, rows[i]["model_id"], want)
		}
	}

	// Unscored suites drop out of the totals (numerator and denominator
	// alike): alpha judged nothing (null total), beta and gamma average
	// their four scored rule suites only — cap_language is not done and must
	// not dilute the total.
	if rows[2]["total_score"] != nil {
		t.Errorf("alpha total_score = %v, want null (nothing judged)", rows[2]["total_score"])
	}
	for _, row := range rows[:2] {
		scores, _ := row["suite_scores"].(map[string]interface{})
		if scores["cap_language"] != nil {
			t.Errorf("model %v cap_language score = %v, want null (suite still running)", row["model_id"], scores["cap_language"])
		}
		if row["total_score"] != 100.0 {
			t.Errorf("model %v total_score = %v, want 100 (mean of the four done rule suites)",
				row["model_id"], row["total_score"])
		}
	}
	assertRowTotals(t, report)

	// No ranking-adjacent data leaks pre-settle: no baseline, no deltas.
	if report["baseline"] != nil {
		t.Errorf("running campaign baseline = %v, want null", report["baseline"])
	}
	for _, row := range rows {
		if row["total_delta"] != nil {
			t.Errorf("model %v total_delta = %v, want null pre-settle", row["model_id"], row["total_delta"])
		}
	}

	// Progress cells: the full 3 x 5 matrix with three of the four states
	// observable — done (settled suites, plus alpha's fully recorded broken
	// cap_language results), running (beta mid-cap_language with 7 of 10
	// judged), pending (gamma untouched in cap_language). The failed state is
	// covered by TestCampaignPartialFailureAggregatesFailed.
	for _, row := range rows {
		for _, key := range []string{"cap_instruction", "cap_reasoning", "cap_coding", "cap_knowledge"} {
			judged := 10
			if row["model_id"] == "alpha-model" {
				judged = 0
			}
			assertCell(t, row, key, "done", judged, 10)
		}
	}
	alpha, beta, gamma := rows[2], rows[0], rows[1]
	assertCell(t, alpha, "cap_language", "done", 0, 10)
	assertCell(t, beta, "cap_language", "running", 7, 10)
	assertCell(t, gamma, "cap_language", "pending", 0, 10)

	// Settle: release the judge and let the sweep finish. The report then
	// serves the full ranked board — beta and gamma tied (ties break by
	// model id), alpha last with a null total, every cell done.
	stub.releaseModel(store.DefaultJudgeModel)
	waitCampaignStatus(t, ts.URL, campaignID, store.CampaignStatusDone)

	settled := getCampaignReport(t, ts.URL, campaignID, "")
	settledRows := reportRows(t, settled)
	if len(settledRows) != 3 {
		t.Fatalf("settled rows = %v, want all three models", settledRows)
	}
	if settledRows[0]["model_id"] != "beta-model" || settledRows[1]["model_id"] != "gamma-model" {
		t.Errorf("settled top ranks = [%v %v], want beta then gamma (tied, model-id break)",
			settledRows[0]["model_id"], settledRows[1]["model_id"])
	}
	if settledRows[2]["model_id"] != "alpha-model" || settledRows[2]["total_score"] != nil {
		t.Errorf("unscored alpha must rank last with null total, got %v", settledRows[2])
	}
	assertRowTotals(t, settled)

	for _, row := range settledRows {
		cells := reportCells(t, row)
		if len(cells) != suiteCount(t, ts.URL) {
			t.Fatalf("model %v has %d cells, want one per suite", row["model_id"], len(cells))
		}
		for _, cell := range cells {
			if cell["status"] != "done" {
				t.Errorf("model %v suite %v cell status = %v, want done",
					row["model_id"], cell["suite_key"], cell["status"])
			}
		}
		// Alpha judged nothing anywhere; the others judged every case.
		if row["model_id"] == "alpha-model" {
			for key, cell := range cellBySuiteKey(t, row) {
				if got := int(cell["judged_cases"].(float64)); got != 0 {
					t.Errorf("alpha suite %s judged = %d, want 0 (all calls broken)", key, got)
				}
			}
			continue
		}
		for _, key := range []string{"cap_instruction", "cap_reasoning", "cap_coding", "cap_knowledge", "cap_language"} {
			assertCell(t, row, key, "done", 10, 10)
		}
	}
}
