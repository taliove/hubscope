package server_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/taliove/hubscope/internal/store"
)

// This file pins the spec 0014 decision-A coverage gate (ticket 91): on a
// settled campaign a model whose AVG(score) is null in any covered suite —
// every case unjudged there, the production "absent valedictorian" shape —
// forfeits its total, its delta and its rank, and sinks below every fully
// judged model. The helpers below are local to this file; shared harness
// primitives (env, stub hub, HTTP seam) are reused untouched.

// waitForSuiteRunStatus polls the campaign detail until the run covering
// suiteKey reaches the wanted status. It exists so a test can hold a model
// broken for exactly one suite of a sweep: keep it broken, wait for that
// suite's run to settle, then unbreak — every later suite judges normally.
func waitForSuiteRunStatus(t *testing.T, base string, campaignID int64, suiteKey, want string) {
	t.Helper()
	suiteID := suiteIDByKey(t, base, suiteKey)
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		campaign := getCampaign(t, base, campaignID)
		for _, run := range campaignRuns(t, campaign) {
			if int64(run["suite_id"].(float64)) == suiteID && run["status"] == want {
				return
			}
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("campaign %d suite %q run did not reach %q in time", campaignID, suiteKey, want)
}

// gateRowOrder extracts the model_id sequence of leaderboard rows.
func gateRowOrder(rows []map[string]interface{}) []string {
	order := make([]string, 0, len(rows))
	for _, row := range rows {
		order = append(order, row["model_id"].(string))
	}
	return order
}

// assertGateOrder fails unless the rows appear in exactly the wanted order.
func assertGateOrder(t *testing.T, rows []map[string]interface{}, want ...string) {
	t.Helper()
	got := gateRowOrder(rows)
	if len(got) != len(want) {
		t.Fatalf("leaderboard order = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("leaderboard order = %v, want %v", got, want)
		}
	}
}

// assertGateCompleteRow pins the gate contract of a fully judged model:
// complete=true, no missing_suites key, and a non-null total.
func assertGateCompleteRow(t *testing.T, row map[string]interface{}) {
	t.Helper()
	complete, ok := row["complete"].(bool)
	if !ok || !complete {
		t.Errorf("model %v: complete = %v (key present=%v), want true", row["model_id"], row["complete"], ok)
	}
	if _, present := row["missing_suites"]; present {
		t.Errorf("model %v: missing_suites = %v on a complete row, want the key absent", row["model_id"], row["missing_suites"])
	}
	if row["total_score"] == nil {
		t.Errorf("model %v: complete row total_score = null, want a number", row["model_id"])
	}
}

// assertGateIncompleteRow pins the gate contract of a model with unjudged
// suites: complete=false, the missing-suite count, and null total/delta —
// an incomplete model never ranks and never compares.
func assertGateIncompleteRow(t *testing.T, row map[string]interface{}, wantMissing int) {
	t.Helper()
	complete, ok := row["complete"].(bool)
	if !ok || complete {
		t.Errorf("model %v: complete = %v (key present=%v), want false", row["model_id"], row["complete"], ok)
	}
	missing, ok := row["missing_suites"].(float64)
	if !ok || int(missing) != wantMissing {
		t.Errorf("model %v: missing_suites = %v, want %d", row["model_id"], row["missing_suites"], wantMissing)
	}
	if row["total_score"] != nil {
		t.Errorf("model %v: incomplete row total_score = %v, want null (no total without full coverage)", row["model_id"], row["total_score"])
	}
	if row["total_delta"] != nil {
		t.Errorf("model %v: incomplete row total_delta = %v, want null (no delta without a total)", row["model_id"], row["total_delta"])
	}
}

// mintShareLinkForCampaign creates a share link over the session client and
// returns its token.
func mintShareLinkForCampaign(t *testing.T, base string, campaignID int64) string {
	t.Helper()
	resp := doPost(t, fmt.Sprintf("%s/api/campaigns/%d/share-links", base, campaignID), nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("POST share link: expected 201, got %d: %s", resp.StatusCode, b)
	}
	var env envelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode share link: %v", err)
	}
	var link map[string]interface{}
	if err := json.Unmarshal(env.Data, &link); err != nil {
		t.Fatalf("unmarshal share link: %v", err)
	}
	token, _ := link["token"].(string)
	if token == "" {
		t.Fatalf("share link token missing: %v", link)
	}
	return token
}

// getSharedReportByToken fetches GET /api/shared-reports/{token} anonymously
// and returns the decoded report.
func getSharedReportByToken(t *testing.T, base, token string) map[string]interface{} {
	t.Helper()
	resp := plainGet(t, base+"/api/shared-reports/"+token)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("GET /api/shared-reports/{token}: expected 200, got %d: %s", resp.StatusCode, b)
	}
	var env envelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode shared report: %v", err)
	}
	var report map[string]interface{}
	if err := json.Unmarshal(env.Data, &report); err != nil {
		t.Fatalf("unmarshal shared report: %v", err)
	}
	return report
}

// TestReportCompletenessGateSettledBoard is the headline scenario: gamma's
// calls fail through the whole cap_instruction run (every case there
// unjudged) while its other four suites judge perfectly — under the pre-gate
// formula its partial total (98+) would have outranked both fully judged
// models (totals ≈ 6), the exact production "absent valedictorian". The gate
// must sink it below them with null total/delta on all three consuming
// endpoints, leave its real per-suite scores visible, keep the complete
// models' relative order untouched, and never leak into the live board.
func TestReportCompletenessGateSettledBoard(t *testing.T) {
	ts, stub, _ := setupEvalEnv(t)
	createEvalModel(t, ts.URL, stub.URL, "alpha-model")
	createEvalModel(t, ts.URL, stub.URL, "beta-model")
	createEvalModel(t, ts.URL, stub.URL, "gamma-model")

	// Batch 1: a fully judged baseline for every model.
	first := triggerFullSweep(t, ts.URL)
	firstID := int64(first["id"].(float64))
	waitCampaignStatus(t, ts.URL, firstID, store.CampaignStatusDone)

	// Batch 2: alpha and beta answer everything wrong (judged — complete,
	// low scores); gamma is broken for exactly the cap_instruction run, so
	// that suite comes back fully unjudged while the rest judge normally.
	stub.markBad("alpha-model", true)
	stub.markBad("beta-model", true)
	stub.markBroken("gamma-model", true)
	second := triggerFullSweep(t, ts.URL)
	secondID := int64(second["id"].(float64))
	waitForSuiteRunStatus(t, ts.URL, secondID, "cap_instruction", "done")
	stub.markBroken("gamma-model", false)
	waitCampaignStatus(t, ts.URL, secondID, store.CampaignStatusDone)

	report := getCampaignReport(t, ts.URL, secondID, "")
	rows := reportRows(t, report)
	// Pre-gate order would have been [gamma, alpha, beta]: gamma's partial
	// total (~98) beats the complete models' ~6. The gate inverts it.
	assertGateOrder(t, rows, "alpha-model", "beta-model", "gamma-model")
	assertGateCompleteRow(t, rows[0])
	assertGateCompleteRow(t, rows[1])
	assertGateIncompleteRow(t, rows[2], 1)

	// The incomplete row keeps its real per-suite scores — only the total,
	// the delta and the rank are forfeited (W7: unjudged stays null, not 0).
	gamma := rowByModel(t, rows, "gamma-model")
	gammaScores, _ := gamma["suite_scores"].(map[string]interface{})
	if gammaScores["cap_instruction"] != nil {
		t.Errorf("gamma cap_instruction = %v, want null (fully unjudged suite)", gammaScores["cap_instruction"])
	}
	if got, ok := gammaScores["cap_reasoning"].(float64); !ok || got <= 0 {
		t.Errorf("gamma cap_reasoning = %v, want its real positive score", gammaScores["cap_reasoning"])
	}

	// Complete models keep their baseline deltas; gamma has none.
	alpha := rowByModel(t, rows, "alpha-model")
	if d := alpha["total_delta"]; d == nil {
		t.Errorf("alpha total_delta = null, want a number (complete on both batches)")
	}

	// Suite-column sorting obeys the same gate: gamma holds the highest
	// cap_reasoning score (100 vs 0/0) yet still ranks last.
	byReasoning := getCampaignReport(t, ts.URL, secondID, "sort=cap_reasoning")
	assertGateOrder(t, reportRows(t, byReasoning), "alpha-model", "beta-model", "gamma-model")

	// The public board and the token-gated shared report serve the identical
	// shape and ranking caliber (spec 0014: three endpoints, one caliber).
	board := getPublicEvalBoard(t, ts.URL)
	boardReport, _ := board["report"].(map[string]interface{})
	if boardReport == nil {
		t.Fatalf("public board report = null, want batch %d", secondID)
	}
	if id := int64(boardReport["id"].(float64)); id != secondID {
		t.Fatalf("public board campaign = %d, want the latest settled batch %d", id, secondID)
	}
	boardRows := reportRows(t, boardReport)
	assertGateOrder(t, boardRows, "alpha-model", "beta-model", "gamma-model")
	assertGateIncompleteRow(t, rowByModel(t, boardRows, "gamma-model"), 1)

	shared := getSharedReportByToken(t, ts.URL, mintShareLinkForCampaign(t, ts.URL, secondID))
	sharedRows := reportRows(t, shared)
	assertGateOrder(t, sharedRows, "alpha-model", "beta-model", "gamma-model")
	assertGateIncompleteRow(t, rowByModel(t, sharedRows, "gamma-model"), 1)

	// Batch 3: everyone judges everywhere again. While the sweep is frozen
	// mid-flight the live board must not carry the gate fields at all — the
	// coverage gate is a settled-batch ranking rule and never leaks into the
	// live half-scored board.
	stub.markBad("alpha-model", false)
	stub.markBad("beta-model", false)
	stub.blockCalls()
	third := triggerFullSweep(t, ts.URL)
	thirdID := int64(third["id"].(float64))
	live := getCampaignReport(t, ts.URL, thirdID, "")
	if live["status"] == store.CampaignStatusDone || live["status"] == "failed" {
		t.Fatalf("batch 3 status = %v while calls blocked, want an unfinished status", live["status"])
	}
	for _, row := range reportRows(t, live) {
		if _, present := row["complete"]; present {
			t.Errorf("live row %v carries complete = %v; the gate must not touch the live board", row["model_id"], row["complete"])
		}
		if _, present := row["missing_suites"]; present {
			t.Errorf("live row %v carries missing_suites; the gate must not touch the live board", row["model_id"])
		}
	}
	stub.release()
	waitCampaignStatus(t, ts.URL, thirdID, store.CampaignStatusDone)

	// Settled batch 3: gamma is complete again, but its delta stays null —
	// the baseline batch's gamma total was gated away, so there is nothing
	// comparable on the other side either.
	third3 := getCampaignReport(t, ts.URL, thirdID, "")
	gamma3 := rowByModel(t, reportRows(t, third3), "gamma-model")
	assertGateCompleteRow(t, gamma3)
	if d := gamma3["total_delta"]; d != nil {
		t.Errorf("gamma total_delta = %v, want null (baseline batch had gamma incomplete)", d)
	}
	alpha3 := rowByModel(t, reportRows(t, third3), "alpha-model")
	if d := alpha3["total_delta"]; d == nil {
		t.Errorf("alpha total_delta = null, want a number (complete on both sides of the baseline)")
	}
}

// TestReportCompletenessGateMultipleIncomplete pins the tail group of a
// settled board: several incomplete models sort among themselves by model_id
// lexicographic order, each carrying its own missing-suite count, and every
// complete model — even the lexicographically last one — outranks them.
func TestReportCompletenessGateMultipleIncomplete(t *testing.T) {
	ts, stub, _ := setupEvalEnv(t)
	// delta-model is created last so it executes last inside each run,
	// which keeps the mid-sweep unbreak windows wide.
	createEvalModel(t, ts.URL, stub.URL, "foxtrot-model")
	createEvalModel(t, ts.URL, stub.URL, "echo-model")
	createEvalModel(t, ts.URL, stub.URL, "delta-model")

	// delta-model misses two suites (cap_instruction + cap_reasoning),
	// echo-model misses one (cap_instruction); foxtrot-model is complete.
	stub.markBroken("delta-model", true)
	stub.markBroken("echo-model", true)
	campaign := triggerFullSweep(t, ts.URL)
	campaignID := int64(campaign["id"].(float64))
	waitForSuiteRunStatus(t, ts.URL, campaignID, "cap_instruction", "done")
	stub.markBroken("echo-model", false)
	waitForSuiteRunStatus(t, ts.URL, campaignID, "cap_reasoning", "done")
	stub.markBroken("delta-model", false)
	waitCampaignStatus(t, ts.URL, campaignID, store.CampaignStatusDone)

	rows := reportRows(t, getCampaignReport(t, ts.URL, campaignID, ""))
	assertGateOrder(t, rows, "foxtrot-model", "delta-model", "echo-model")
	assertGateCompleteRow(t, rowByModel(t, rows, "foxtrot-model"))
	assertGateIncompleteRow(t, rowByModel(t, rows, "delta-model"), 2)
	assertGateIncompleteRow(t, rowByModel(t, rows, "echo-model"), 1)
}

// TestReportCompletenessGateAllIncomplete pins the all-incomplete edge: the
// report stays a normal 200 with the rows present — every total/delta null,
// every row flagged incomplete — instead of erroring or emptying the board.
func TestReportCompletenessGateAllIncomplete(t *testing.T) {
	ts, stub, _ := setupEvalEnv(t)
	zetaID := createEvalModel(t, ts.URL, stub.URL, "zeta-model")
	stub.markBroken("zeta-model", true)

	runID := triggerEval(t, ts.URL, suiteIDByKey(t, ts.URL, "cap_instruction"), zetaID)
	run := waitEvalDone(t, ts.URL, runID)
	campaignID := int64(run["campaign_id"].(float64))
	waitCampaignStatus(t, ts.URL, campaignID, store.CampaignStatusDone)

	report := getCampaignReport(t, ts.URL, campaignID, "")
	rows := reportRows(t, report)
	if len(rows) != 1 {
		t.Fatalf("all-incomplete report rows = %v, want the single broken model's row", rows)
	}
	assertGateIncompleteRow(t, rows[0], 1)
}
