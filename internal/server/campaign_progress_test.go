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
// out of the totals (numerator and denominator alike), and no baseline/delta
// information leaks before the
// batch settles. After the sweep completes, the same endpoint serves the
// full ranked board with every cell done.
//
// Scenario (post-cutover, ticket 99; re-derived for the GH #169
// model-major cell order): a full sweep over three models where gamma's
// answer calls all fail (unjudged results) and gamma is frozen mid-ifeval.
// Cells execute model by model in creation order, each model walking the
// five benchmark suites in bank order (mmlu, agieval_zh, gsm8k, cruxeval,
// ifeval), so the broken model must run LAST — its dead cells would trip
// the GH #153 all-dead abort if no live cell had completed first. Alpha
// and beta finish every suite before gamma starts, so at the freeze point
// the four earlier runs are done (scores exist), ifeval is mid-flight,
// and gamma has zero judged cases anywhere. A broken cell burns exactly
// ten calls (5 cases x answer+retry before the GH #153 circuit opens), so
// gating gamma after 42 calls freezes it at its second ifeval case. The
// stub's default answers score 0 on every benchmark case but stay judged,
// which is exactly the coverage this grid asserts.
func TestCampaignReportProgressGrid(t *testing.T) {
	// Async eval: observes the running report with gamma frozen mid-ifeval
	// (blocked at its 43rd call); drained by releaseModel +
	// waitCampaignStatus(done).
	ts, stub, _ := setupAsyncEvalEnv(t)
	createEvalModel(t, ts.URL, stub.URL, "alpha-model")
	createEvalModel(t, ts.URL, stub.URL, "beta-model")
	createEvalModel(t, ts.URL, stub.URL, "gamma-model")
	stub.markBroken("gamma-model", true)
	// Serial cell order (GH #26 pool at 1, GH #169 model-major): gamma's
	// call count gates suite boundaries only under a deterministic
	// model→suite order.
	setEvalConcurrency(t, ts.URL, 1)
	stub.resetCalls()
	stub.blockModelAfter("gamma-model", 42)
	t.Cleanup(func() { stub.releaseModel("gamma-model") })

	campaign := triggerFullSweep(t, ts.URL)
	campaignID := int64(campaign["id"].(float64))
	// Gamma's 43rd call being recorded proves the freeze point: gamma is
	// blocked on its second ifeval case, the four earlier suites settled
	// for everyone.
	waitFor(t, "gamma's 43rd call reaching the stub", func() bool {
		return stub.callTotal("gamma-model") >= 43
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
		t.Errorf("progress.done = %d, want 4 (earlier suites settled)", got)
	}
	if got := int(progress["running"].(float64)); got != 1 {
		t.Errorf("progress.running = %d, want 1 (ifeval mid-flight)", got)
	}

	// The live board ranks by the half-scored total descending (GH #40):
	// alpha and beta both total 0 (their four done suites, all scored 0 by
	// the default answers) and tie into model-id order; gamma judged
	// nothing anywhere and sinks to the bottom with a null total.
	rows := reportRows(t, report)
	if len(rows) != 3 {
		t.Fatalf("running report rows = %v, want alpha, beta, gamma", rows)
	}
	for i, want := range []string{"alpha-model", "beta-model", "gamma-model"} {
		if rows[i]["model_id"] != want {
			t.Errorf("live board row %d = %v, want %s (partial total desc, null sunk)", i, rows[i]["model_id"], want)
		}
	}

	// Unscored suites drop out of the totals (numerator and denominator
	// alike): gamma judged nothing (null total), alpha and beta average
	// their four scored suites only — ifeval is not done and must not
	// dilute the total.
	if rows[2]["total_score"] != nil {
		t.Errorf("gamma total_score = %v, want null (nothing judged)", rows[2]["total_score"])
	}
	for _, row := range rows[:2] {
		scores, _ := row["suite_scores"].(map[string]interface{})
		if scores["ifeval"] != nil {
			t.Errorf("model %v ifeval score = %v, want null (suite still running)", row["model_id"], scores["ifeval"])
		}
		if row["total_score"] != 0.0 {
			t.Errorf("model %v total_score = %v, want 0 (mean of the four done suites, all scored 0 by the default answers)",
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

	// Progress cells: the full 3 x 5 matrix — done (settled suites,
	// including gamma's fully recorded broken results), running (gamma
	// mid-ifeval with zero judged). Pending cells are covered by
	// TestSharedReportHidesUnfinishedBoard (a model the sweep has not
	// reached), the failed state by
	// TestCampaignPartialFailureAggregatesFailed.
	alpha, beta, gamma := rows[0], rows[1], rows[2]
	for _, key := range []string{"mmlu", "agieval_zh", "gsm8k", "cruxeval"} {
		assertCell(t, alpha, key, "done", 100, 100)
		assertCell(t, beta, key, "done", 100, 100)
		assertCell(t, gamma, key, "done", 0, 100)
	}
	// Alpha's and beta's ifeval results are complete inside the still-
	// running run: their cells read done while gamma's is mid-flight.
	assertCell(t, alpha, "ifeval", "done", 100, 100)
	assertCell(t, beta, "ifeval", "done", 100, 100)
	assertCell(t, gamma, "ifeval", "running", 0, 100)

	// Settle: release gamma and let the sweep finish. The report then serves
	// the full ranked board — alpha and beta tied at 0 (ties break by model
	// id), gamma last with a null total, every cell done.
	stub.releaseModel("gamma-model")
	waitCampaignStatus(t, ts.URL, campaignID, store.CampaignStatusDone)

	settled := getCampaignReport(t, ts.URL, campaignID, "")
	settledRows := reportRows(t, settled)
	if len(settledRows) != 3 {
		t.Fatalf("settled rows = %v, want all three models", settledRows)
	}
	if settledRows[0]["model_id"] != "alpha-model" || settledRows[1]["model_id"] != "beta-model" {
		t.Errorf("settled top ranks = [%v %v], want alpha then beta (tied, model-id break)",
			settledRows[0]["model_id"], settledRows[1]["model_id"])
	}
	if settledRows[2]["model_id"] != "gamma-model" || settledRows[2]["total_score"] != nil {
		t.Errorf("unscored gamma must rank last with null total, got %v", settledRows[2])
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
		// Gamma judged nothing anywhere; the others judged every case.
		if row["model_id"] == "gamma-model" {
			for key, cell := range cellBySuiteKey(t, row) {
				if got := int(cell["judged_cases"].(float64)); got != 0 {
					t.Errorf("gamma suite %s judged = %d, want 0 (all calls broken)", key, got)
				}
			}
			continue
		}
		for _, key := range benchmarkRotation {
			assertCell(t, row, key, "done", 100, 100)
		}
	}
}
