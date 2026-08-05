package server_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/taliove/hubscope/internal/store"
)

// This file pins the spec 0014 decision-A coverage gate (ticket 91): on a
// settled campaign a model whose AVG(score) is null in any covered suite —
// every case unjudged there, the production "absent valedictorian" shape —
// forfeits its total, its delta and its rank, and sinks below every fully
// judged model. The helpers below are local to this file; shared harness
// primitives (env, stub hub, HTTP seam) are reused untouched.

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
// calls fail through the whole mmlu run (every case there unjudged) while
// its other four suites judge perfectly — under the pre-gate formula its
// partial total (100) would have outranked both fully judged models
// (totals 0), the exact production "absent valedictorian". The gate must
// sink it below them with null total/delta on all three consuming
// endpoints, leave its real per-suite scores visible, keep the complete
// models' relative order untouched, and never leak into the live board.
func TestReportCompletenessGateSettledBoard(t *testing.T) {
	// Async eval: batch 3 observes the live half-scored board (all calls
	// blocked); drained by stub.release + waitCampaignStatus(done).
	ts, stub, db := setupAsyncEvalEnv(t)
	createEvalModel(t, ts.URL, stub.URL, "alpha-model")
	createEvalModel(t, ts.URL, stub.URL, "beta-model")
	createEvalModel(t, ts.URL, stub.URL, "gamma-model")
	// One custom exact-rule case per rotation suite: normal answers score
	// 100, bad answers score 0 (judged), broken calls stay unjudged.
	installCustomBank(t, ts.URL, db, oneCasePerSuite())
	// Serial cell order (GH #26 pool at 1, GH #169 model-major): each
	// model's broken stretch is contiguous, so "gamma broken until its
	// second suite starts" maps to exactly the mmlu suite when gamma's own
	// gate freezes it at agieval_zh's first case.
	setEvalConcurrency(t, ts.URL, 1)

	// Batch 1: a fully judged baseline for every model.
	first := triggerFullSweep(t, ts.URL)
	firstID := int64(first["id"].(float64))
	waitCampaignStatus(t, ts.URL, firstID, store.CampaignStatusDone)

	// Batch 2: alpha and beta answer everything wrong (judged — complete,
	// low scores); gamma is broken for exactly the mmlu suite (its first
	// cell under model-major order), so that suite comes back fully
	// unjudged while the rest judge normally. The freeze is deterministic:
	// gamma runs last, and a broken case costs two calls (answer +
	// immediate retry, GH #27), so gating gamma after two calls freezes it
	// at agieval_zh's first case, where the test lifts the break before
	// releasing (the stub reads the broken flag at call time, so gamma
	// must be unbroken before its second-suite call is made, not merely
	// before it is answered).
	stub.markBad("alpha-model", true)
	stub.markBad("beta-model", true)
	// Case-broken (GH #174): gamma passes the probe gate and fails at case
	// time — the gate would exclude a fully broken model outright.
	stub.markCaseBroken("gamma-model", true)
	stub.resetCalls()
	// 3 probe rounds + one broken mmlu case (2 attempts): the 6th call is
	// agieval_zh's first case.
	stub.blockModelAfter("gamma-model", 5)
	t.Cleanup(func() { stub.releaseModel("gamma-model") })
	second := triggerFullSweep(t, ts.URL)
	secondID := int64(second["id"].(float64))
	waitFor(t, "gamma frozen before its second-suite call", func() bool {
		return stub.callTotal("gamma-model") >= 6
	})
	stub.markCaseBroken("gamma-model", false)
	stub.releaseModel("gamma-model")
	waitCampaignStatus(t, ts.URL, secondID, store.CampaignStatusDone)

	report := getCampaignReport(t, ts.URL, secondID, "")
	rows := reportRows(t, report)
	// Pre-gate order would have been [gamma, alpha, beta]: gamma's partial
	// total (100) beats the complete models' 0. The gate inverts it.
	assertGateOrder(t, rows, "alpha-model", "beta-model", "gamma-model")
	assertGateCompleteRow(t, rows[0])
	assertGateCompleteRow(t, rows[1])
	assertGateIncompleteRow(t, rows[2], 1)

	// The incomplete row keeps its real per-suite scores — only the total,
	// the delta and the rank are forfeited (W7: unjudged stays null, not 0).
	gamma := rowByModel(t, rows, "gamma-model")
	gammaScores, _ := gamma["suite_scores"].(map[string]interface{})
	if gammaScores["mmlu"] != nil {
		t.Errorf("gamma mmlu = %v, want null (fully unjudged suite)", gammaScores["mmlu"])
	}
	if got, ok := gammaScores["gsm8k"].(float64); !ok || got <= 0 {
		t.Errorf("gamma gsm8k = %v, want its real positive score", gammaScores["gsm8k"])
	}

	// Complete models keep their baseline deltas; gamma has none.
	alpha := rowByModel(t, rows, "alpha-model")
	if d := alpha["total_delta"]; d == nil {
		t.Errorf("alpha total_delta = null, want a number (complete on both batches)")
	}

	// Suite-column sorting obeys the same gate: gamma holds the highest
	// gsm8k score (100 vs 0/0) yet still ranks last.
	byReasoning := getCampaignReport(t, ts.URL, secondID, "sort=gsm8k")
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
	// Async eval: steps the sweep through mid-flight unbreak windows. The
	// windows are gated, not polled: under the GH #169 model-major order
	// each model's broken stretch is contiguous, so echo is frozen at its
	// second suite's first case and delta at its third suite's first case,
	// and neither can advance past the boundary while its broken flag flips
	// (ticket 100). Drained by releaseModel + waitCampaignStatus(done).
	ts, stub, db := setupAsyncEvalEnv(t)
	// Creation order is the cell order: foxtrot runs first (complete),
	// delta last.
	createEvalModel(t, ts.URL, stub.URL, "foxtrot-model")
	createEvalModel(t, ts.URL, stub.URL, "echo-model")
	createEvalModel(t, ts.URL, stub.URL, "delta-model")
	installCustomBank(t, ts.URL, db, oneCasePerSuite())
	// Serial cell order (GH #26 pool at 1, GH #169 model-major): models
	// run strictly one after another, so each model's call count gates its
	// own suite boundaries.
	setEvalConcurrency(t, ts.URL, 1)

	// One custom exact-rule case per rotation suite (oneCasePerSuite). A
	// broken case costs two calls (answer + immediate retry, GH #27), so a
	// broken suite costs each model exactly two calls. Case-broken
	// (GH #174): both pass the probe gate and fail at case time.
	stub.markCaseBroken("delta-model", true)
	stub.markCaseBroken("echo-model", true)
	stub.resetCalls()
	// Both gates armed up front (count-based, no slip window): echo freezes
	// at agieval_zh's first case (3 probe rounds + 2 broken-case calls),
	// delta at gsm8k's first case (3 probe rounds + 4).
	stub.blockModelAfter("echo-model", 5)
	stub.blockModelAfter("delta-model", 7)
	t.Cleanup(func() { stub.releaseModel("echo-model") })
	t.Cleanup(func() { stub.releaseModel("delta-model") })

	campaign := triggerFullSweep(t, ts.URL)
	campaignID := int64(campaign["id"].(float64))
	// echo recovers after its broken mmlu cell: unbreak while it is frozen
	// at agieval_zh's first case, so exactly one suite stays missing.
	waitFor(t, "echo-model frozen at agieval_zh", func() bool {
		return stub.callTotal("echo-model") >= 6
	})
	stub.markCaseBroken("echo-model", false)
	stub.releaseModel("echo-model")
	// delta recovers after two broken suites: unbreak while it is frozen at
	// gsm8k's first case, so exactly two suites stay missing.
	waitFor(t, "delta-model frozen at gsm8k", func() bool {
		return stub.callTotal("delta-model") >= 8
	})
	stub.markCaseBroken("delta-model", false)
	stub.releaseModel("delta-model")
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
	// Case-broken (GH #174): the gate admits it, every case fails — an
	// all-null row set with the model present.
	stub.markCaseBroken("zeta-model", true)

	runID := triggerEval(t, ts.URL, suiteIDByKey(t, ts.URL, "gsm8k"), zetaID)
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
