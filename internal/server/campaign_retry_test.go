package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/taliove/hubscope/internal/server"
	"github.com/taliove/hubscope/internal/store"
)

// postRetryFailed calls POST /api/campaigns/{id}/retry-failed (GH #28).
func postRetryFailed(t *testing.T, base string, campaignID int64) *http.Response {
	t.Helper()
	return doPost(t, fmt.Sprintf("%s/api/campaigns/%d/retry-failed", base, campaignID), nil)
}

// runDetail fetches GET /api/evals/{id}.
func runDetail(t *testing.T, base string, runID int64) map[string]interface{} {
	t.Helper()
	resp := doGet(t, fmt.Sprintf("%s/api/evals/%d", base, runID))
	defer resp.Body.Close()
	var env envelope
	_ = json.NewDecoder(resp.Body).Decode(&env)
	var run map[string]interface{}
	_ = json.Unmarshal(env.Data, &run)
	return run
}

// campaignReportField fetches the campaign report and returns it decoded.
func campaignReport(t *testing.T, base string, campaignID int64) map[string]interface{} {
	t.Helper()
	resp := doGet(t, fmt.Sprintf("%s/api/campaigns/%d/report", base, campaignID))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET report: expected 200, got %d", resp.StatusCode)
	}
	var env envelope
	_ = json.NewDecoder(resp.Body).Decode(&env)
	var report map[string]interface{}
	_ = json.Unmarshal(env.Data, &report)
	return report
}

// campaignIDOfRun resolves the campaign of a run detail payload.
func campaignIDOfRun(t *testing.T, run map[string]interface{}) int64 {
	t.Helper()
	id, ok := run["campaign_id"].(float64)
	if !ok || id == 0 {
		t.Fatalf("run campaign_id missing: %v", run)
	}
	return int64(id)
}

// TestRetryFailedRefillsNullScoresOnly covers the happy path (GH #28): a
// settled batch with a broken model retries its failed cells, scored results
// stay byte-identical throughout (W7), and once the hub recovers a second
// retry fills every null — the batch ends fully judged (complete=true).
func TestRetryFailedRefillsNullScoresOnly(t *testing.T) {
	ts, stub, db := setupEvalEnv(t)
	smartID := createEvalModel(t, ts.URL, stub.URL, "smart-model")
	brokenID := createEvalModel(t, ts.URL, stub.URL, "broken-model")
	// Case-broken (GH #174): the gate admits the model, cases fail — the
	// null-score rows the retry refills.
	stub.markCaseBroken("broken-model", true)
	// Custom exact-rule bank: the default stub answer scores 1 once the
	// model recovers, so the refill is observable as null -> 1.
	installCustomBank(t, ts.URL, db, oneCasePerSuite())
	suiteID := suiteIDByKey(t, ts.URL, "mmlu")

	runID := triggerEval(t, ts.URL, suiteID, smartID, brokenID)
	run := waitEvalDone(t, ts.URL, runID)
	campaignID := campaignIDOfRun(t, run)
	waitCampaignStatus(t, ts.URL, campaignID, "done")

	// Every broken-model case failed (twice, via the GH #27 answer retry) and
	// landed as a null score.
	before := runDetail(t, ts.URL, runID)
	smartBefore := resultsByModel(before, "smart-model")
	brokenBefore := resultsByModel(before, "broken-model")
	if len(smartBefore) == 0 || len(brokenBefore) == 0 {
		t.Fatalf("expected results for both models, got smart=%d broken=%d", len(smartBefore), len(brokenBefore))
	}
	for _, r := range brokenBefore {
		if r["score"] != nil {
			t.Fatalf("broken case %v score = %v, want null before retry", r["case_id"], r["score"])
		}
	}

	// The report exposes the failure count driving the retry button.
	report := campaignReport(t, ts.URL, campaignID)
	if got := int(report["failed_results"].(float64)); got != len(brokenBefore) {
		t.Errorf("report failed_results = %d, want %d", got, len(brokenBefore))
	}

	// Retry while the model is still broken: accepted, re-settles to done,
	// failures remain — and nothing scored moved a byte.
	resp := postRetryFailed(t, ts.URL, campaignID)
	resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("retry-failed: expected 202, got %d", resp.StatusCode)
	}
	waitCampaignStatus(t, ts.URL, campaignID, "done")
	mid := runDetail(t, ts.URL, runID)
	if mid["status"] != "done" {
		t.Errorf("run status after a retry that failed again = %v, want done "+
			"(null scores are results, not run failure — GH #39)", mid["status"])
	}
	if report := campaignReport(t, ts.URL, campaignID); int(report["failed_results"].(float64)) != len(brokenBefore) {
		t.Errorf("failed_results after a retry that failed again = %v, want %d remaining",
			report["failed_results"], len(brokenBefore))
	}
	if !reflect.DeepEqual(smartBefore, resultsByModel(mid, "smart-model")) {
		t.Error("scored results must stay byte-identical across a retry (W7)")
	}

	// The model recovers: the second retry fills every null, again leaving
	// the scored rows untouched.
	stub.markCaseBroken("broken-model", false)
	resp = postRetryFailed(t, ts.URL, campaignID)
	resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("second retry-failed: expected 202, got %d", resp.StatusCode)
	}
	final := waitCampaignStatus(t, ts.URL, campaignID, "done")
	if final["status"] != "done" {
		t.Fatalf("campaign status after retry = %v, want done", final["status"])
	}

	after := runDetail(t, ts.URL, runID)
	if !reflect.DeepEqual(smartBefore, resultsByModel(after, "smart-model")) {
		t.Error("scored results must stay byte-identical across the refill retry (W7)")
	}
	for _, r := range resultsByModel(after, "broken-model") {
		if r["score"] != 1.0 {
			t.Errorf("retried broken case %v score = %v, want 1", r["case_id"], r["score"])
		}
	}

	// The batch is now fully judged: no failures left, every row complete.
	report = campaignReport(t, ts.URL, campaignID)
	if got := report["failed_results"].(float64); got != 0 {
		t.Errorf("report failed_results after refill = %v, want 0", got)
	}
	for _, row := range report["rows"].([]interface{}) {
		rm := row.(map[string]interface{})
		if rm["complete"] != true {
			t.Errorf("row %v complete = %v, want true after refill", rm["model_id"], rm["complete"])
		}
	}

	// Nothing left to retry: a third attempt conflicts.
	resp = postRetryFailed(t, ts.URL, campaignID)
	resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("retry without failures: expected 409, got %d", resp.StatusCode)
	}
}

// TestRetryFailedRequiresSettledCampaign rejects the retry while the batch
// is still running (GH #28: only done/failed batches retry).
func TestRetryFailedRequiresSettledCampaign(t *testing.T) {
	// Async eval: observes the mid-run 409 rejection (all calls blocked);
	// drained by stub.release + waitCampaignStatus(done).
	ts, stub, _ := setupAsyncEvalEnv(t)
	createEvalModel(t, ts.URL, stub.URL, "smart-model")
	stub.resetCalls()
	stub.blockCalls()
	t.Cleanup(stub.release)

	campaign := triggerFullSweep(t, ts.URL)
	campaignID := int64(campaign["id"].(float64))
	waitFor(t, "first sweep call reaching the stub", func() bool {
		return stub.sawModel("smart-model")
	})

	resp := postRetryFailed(t, ts.URL, campaignID)
	resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("retry on running campaign: expected 409, got %d", resp.StatusCode)
	}

	stub.release()
	waitCampaignStatus(t, ts.URL, campaignID, "done")
}

// TestRetryFailedGuards covers the remaining rejection paths (GH #28):
// unknown campaign → 404; anonymous → 401; settled batch without failures →
// 409.
func TestRetryFailedGuards(t *testing.T) {
	ts, stub, _ := setupEvalEnv(t)
	smartID := createEvalModel(t, ts.URL, stub.URL, "smart-model")
	suiteID := suiteIDByKey(t, ts.URL, "mmlu")

	runID := triggerEval(t, ts.URL, suiteID, smartID)
	run := waitEvalDone(t, ts.URL, runID)
	campaignID := campaignIDOfRun(t, run)
	waitCampaignStatus(t, ts.URL, campaignID, "done")

	resp := postRetryFailed(t, ts.URL, 999999)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("unknown campaign: expected 404, got %d", resp.StatusCode)
	}

	// A clean sweep has no failures to retry.
	resp = postRetryFailed(t, ts.URL, campaignID)
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("no-failure campaign: expected 409, got %d (%s)", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), "no failed results") {
		t.Errorf("409 body should explain the conflict, got %s", body)
	}

	// Anonymous callers never reach the handler.
	req, _ := http.NewRequest("POST",
		fmt.Sprintf("%s/api/campaigns/%d/retry-failed", ts.URL, campaignID),
		bytes.NewReader(nil))
	anon, err := (&http.Client{}).Do(req)
	if err != nil {
		t.Fatalf("anonymous POST: %v", err)
	}
	anon.Body.Close()
	if anon.StatusCode != http.StatusUnauthorized {
		t.Errorf("anonymous retry: expected 401, got %d", anon.StatusCode)
	}
}

// TestRetryFailedKeepsScoredBytesOnFailedModel is the failAllCases variant:
// a model with no enabled endpoint records failed results for every case;
// the retry must re-evaluate them without touching the other model's scored
// rows (GH #28 store guarantee: the delete is hardcoded to score IS NULL).
func TestRetryFailedCoversSetupFailureCells(t *testing.T) {
	ts, stub, _ := setupEvalEnv(t)
	smartID := createEvalModel(t, ts.URL, stub.URL, "smart-model")
	ghostID := createEvalModel(t, ts.URL, stub.URL, "ghost-model")
	suiteID := suiteIDByKey(t, ts.URL, "mmlu")

	// Disable the ghost model's endpoints so its whole cell fails at setup.
	resp := doGet(t, ts.URL+"/api/models")
	var env envelope
	_ = json.NewDecoder(resp.Body).Decode(&env)
	resp.Body.Close()
	var models []map[string]interface{}
	_ = json.Unmarshal(env.Data, &models)
	for _, m := range models {
		mm := m
		if int64(mm["id"].(float64)) != ghostID {
			continue
		}
		eps, _ := mm["endpoints"].([]interface{})
		for _, ep := range eps {
			epm := ep.(map[string]interface{})
			epID := int64(epm["id"].(float64))
			if en, _ := epm["enabled"].(bool); en {
				r := doPatch(t, fmt.Sprintf("%s/api/endpoints/%d", ts.URL, epID), map[string]interface{}{"enabled": false})
				r.Body.Close()
			}
		}
	}

	runID := triggerEval(t, ts.URL, suiteID, smartID, ghostID)
	run := waitEvalDone(t, ts.URL, runID)
	campaignID := campaignIDOfRun(t, run)
	waitCampaignStatus(t, ts.URL, campaignID, "done")

	before := runDetail(t, ts.URL, runID)
	if got := resultsByModel(before, "ghost-model"); len(got) == 0 {
		t.Fatal("expected failed result rows for the ghost model")
	}
	for _, r := range resultsByModel(before, "ghost-model") {
		if r["score"] != nil {
			t.Fatalf("ghost case %v score = %v, want null", r["case_id"], r["score"])
		}
	}

	report := campaignReport(t, ts.URL, campaignID)
	if got := report["failed_results"].(float64); got == 0 {
		t.Error("expected failed_results > 0 for the ghost model's failed cell")
	}

	// Retry: the ghost cell fails again at setup (still no enabled endpoint),
	// its null rows are re-recorded, and the batch re-settles.
	resp = postRetryFailed(t, ts.URL, campaignID)
	resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("retry-failed: expected 202, got %d", resp.StatusCode)
	}
	waitCampaignStatus(t, ts.URL, campaignID, "done")

	after := runDetail(t, ts.URL, runID)
	if !reflect.DeepEqual(resultsByModel(before, "smart-model"), resultsByModel(after, "smart-model")) {
		t.Error("smart-model scored results must stay byte-identical (W7)")
	}
	if got := resultsByModel(after, "ghost-model"); len(got) == 0 {
		t.Error("ghost model must keep failed result rows after the retry")
	}
	for _, r := range resultsByModel(after, "ghost-model") {
		if r["score"] != nil {
			t.Errorf("ghost case %v score = %v, want still null", r["case_id"], r["score"])
		}
	}
}

// stageNullResult records one null-score (failed) result for the model — a
// staged retry unit (GH #28) in the production shape of GH #39.
// enabledCaseIDs returns the ids of a suite's enabled cases from its API
// payload, in listing order.
func enabledCaseIDs(t *testing.T, suite map[string]interface{}) []int64 {
	t.Helper()
	var ids []int64
	for _, c := range suite["cases"].([]interface{}) {
		cm := c.(map[string]interface{})
		if en, _ := cm["enabled"].(bool); en {
			ids = append(ids, int64(cm["id"].(float64)))
		}
	}
	if len(ids) == 0 {
		t.Fatalf("suite %v has no enabled cases", suite["key"])
	}
	return ids
}

func stageNullResult(t *testing.T, db *store.DB, runID, modelDBID int64, modelID string, caseID int64) {
	t.Helper()
	if _, err := db.CreateEvalResult(store.EvalResult{
		EvalRunID: runID, ModelDBID: modelDBID, ModelID: modelID,
		CaseID: caseID, LatencyMs: 10,
	}); err != nil {
		t.Fatalf("create null result: %v", err)
	}
}

// TestRetryFailedMigratesFailedRunThroughStateChain covers GH #39: a
// settled-failed batch's retry must move the failed run through the real
// state machine — failed → running at reopen (the grid stops showing the
// stale failure), running → done once its failed cells are re-judged, so the
// campaign re-settles to done. The clean sibling done run is never touched.
func TestRetryFailedMigratesFailedRunThroughStateChain(t *testing.T) {
	// Async eval: observes the mid-retry state chain (reopened run running,
	// clean sibling untouched) while the stub gate holds the first retried
	// call; drained by stub.release + waitCampaignStatus(done).
	ts, stub, db := setupAsyncEvalEnv(t)
	modelDBID := createEvalModel(t, ts.URL, stub.URL, "smart-model")

	// Custom exact-rule bank (two cases in mmlu): the default stub answer
	// scores 1, so the retried nulls refill as 1.0.
	installCustomBank(t, ts.URL, db, map[string]int{"mmlu": 2})
	instruction := suiteByKey(t, ts.URL, "mmlu")
	reasoning := suiteByKey(t, ts.URL, "agieval_zh")
	instructionID := int64(instruction["id"].(float64))
	reasoningID := int64(reasoning["id"].(float64))
	// The suites API lists retired cases alongside enabled ones; the custom
	// bank's live cases are the enabled tail.
	customIDs := enabledCaseIDs(t, instruction)
	case1, case2 := customIDs[0], customIDs[1]

	// Stage the production shape of GH #39: a failed batch whose failed run
	// holds two null-score results, next to a clean done run.
	now := time.Now()
	campaign, err := db.CreateCampaign("manual", []int64{modelDBID}, now)
	if err != nil {
		t.Fatalf("create campaign: %v", err)
	}
	runA, err := db.CreateEvalRun(campaign.ID, instructionID, "manual", store.DefaultJudgeModel)
	if err != nil {
		t.Fatalf("create failed run: %v", err)
	}
	stageNullResult(t, db, runA.ID, modelDBID, "smart-model", case1)
	stageNullResult(t, db, runA.ID, modelDBID, "smart-model", case2)
	if err := db.FinishEvalRun(runA.ID, "failed", now); err != nil {
		t.Fatalf("finish run A failed: %v", err)
	}
	runBID := presetScoredRun(t, db, campaign.ID, reasoningID, modelDBID, "smart-model", firstCaseID(t, reasoning), 0.8)
	if err := db.SettleCampaign(campaign.ID, now); err != nil {
		t.Fatalf("settle campaign: %v", err)
	}

	// Sanity: the batch settled failed, carrying two failed results.
	if got := getCampaign(t, ts.URL, campaign.ID)["status"]; got != "failed" {
		t.Fatalf("staged campaign status = %v, want failed", got)
	}
	report := campaignReport(t, ts.URL, campaign.ID)
	if got := int(report["failed_results"].(float64)); got != 2 {
		t.Fatalf("staged failed_results = %d, want 2", got)
	}
	runBBefore := runDetail(t, ts.URL, runBID)

	stub.resetCalls()
	stub.blockCalls()
	t.Cleanup(stub.release)

	resp := postRetryFailed(t, ts.URL, campaign.ID)
	resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("retry-failed: expected 202, got %d", resp.StatusCode)
	}

	// Mid-retry: the retried run rejoined execution (no stale failure on the
	// grid), the clean sibling kept its terminal state byte-for-byte.
	mid := getCampaign(t, ts.URL, campaign.ID)
	if mid["status"] != "running" {
		t.Errorf("campaign status mid-retry = %v, want running", mid["status"])
	}
	progress := campaignProgress(t, mid)
	if got := int(progress["running"].(float64)); got != 1 {
		t.Errorf("mid-retry running runs = %d, want 1 (the retried run)", got)
	}
	if got := runDetail(t, ts.URL, runA.ID)["status"]; got != "running" {
		t.Errorf("retried run status mid-retry = %v, want running", got)
	}
	if runBMid := runDetail(t, ts.URL, runBID); !reflect.DeepEqual(runBBefore, runBMid) {
		t.Error("clean done run must stay byte-identical through the retry (GH #39 guard)")
	}

	stub.release()
	waitCampaignStatus(t, ts.URL, campaign.ID, "done")

	// Settled: the retried run finished done, every failed cell re-judged.
	runAAfter := runDetail(t, ts.URL, runA.ID)
	if runAAfter["status"] != "done" {
		t.Errorf("retried run status after settle = %v, want done", runAAfter["status"])
	}
	for _, r := range resultsByModel(runAAfter, "smart-model") {
		if r["score"] != 1.0 {
			t.Errorf("retried case %v score = %v, want 1", r["case_id"], r["score"])
		}
	}
	if runBAfter := runDetail(t, ts.URL, runBID); !reflect.DeepEqual(runBBefore, runBAfter) {
		t.Error("clean done run must stay byte-identical after the retry (GH #39 guard)")
	}
	report = campaignReport(t, ts.URL, campaign.ID)
	if got := report["failed_results"].(float64); got != 0 {
		t.Errorf("failed_results after refill = %v, want 0", got)
	}
}

// TestRetryFailedCancelFailsRunKeepsUntouchedNulls covers GH #39's
// cancellation semantics, mirroring the normal executor (GH #26): a retry
// whose context is canceled finishes the runs of every cell that never
// started as failed — the campaign re-settles failed — while cases the
// interrupted cell never reached keep their null rows and stay retryable.
// The HTTP handler passes context.Background() (never canceled), so the test
// drives the executor with an injected context, the same pattern as
// TestCampaignFailedWhenBatchAborted; every observation stays on the HTTP
// surface.
func TestRetryFailedCancelFailsRunKeepsUntouchedNulls(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "retry-cancel.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	seedTestUser(t, db)

	stub := newEvalStubHub()
	t.Cleanup(stub.Close)
	// The retry executor is driven directly (asynchronous by design);
	// drain = cancel + wait for the executor, inside which RetryFailedResults
	// runs synchronously (ticket 100). Discovery stays inline.
	srv := server.New(db, server.WithRateLimits(server.RateLimits{}), server.WithSyncDiscovery())
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	// One worker: the first cell blocks inside its second case, so cancelling
	// before release makes the second model's cell observe the canceled
	// context deterministically (the batch-abort freeze point).
	if err := db.SetSettingInt(store.SettingEvalConcurrency, 1); err != nil {
		t.Fatalf("set eval_concurrency: %v", err)
	}
	smartID := createEvalModel(t, ts.URL, stub.URL, "smart-model")
	twoID := createEvalModel(t, ts.URL, stub.URL, "chat-two")

	// Custom exact-rule bank (two cases in mmlu): the default stub answer
	// scores 1, so the case judged before the cancel lands as 1.0.
	installCustomBank(t, ts.URL, db, map[string]int{"mmlu": 2})
	instruction := suiteByKey(t, ts.URL, "mmlu")
	instructionID := int64(instruction["id"].(float64))
	// The suites API lists retired cases alongside enabled ones; the custom
	// bank's live cases are the enabled tail.
	customIDs := enabledCaseIDs(t, instruction)
	case1, case2 := customIDs[0], customIDs[1]

	now := time.Now()
	campaign, err := db.CreateCampaign("manual", []int64{smartID, twoID}, now)
	if err != nil {
		t.Fatalf("create campaign: %v", err)
	}
	run, err := db.CreateEvalRun(campaign.ID, instructionID, "manual", store.DefaultJudgeModel)
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	for _, m := range []struct {
		dbID int64
		id   string
	}{{smartID, "smart-model"}, {twoID, "chat-two"}} {
		stageNullResult(t, db, run.ID, m.dbID, m.id, case1)
		stageNullResult(t, db, run.ID, m.dbID, m.id, case2)
	}
	if err := db.FinishEvalRun(run.ID, "failed", now); err != nil {
		t.Fatalf("finish run failed: %v", err)
	}
	if err := db.SettleCampaign(campaign.ID, now); err != nil {
		t.Fatalf("settle campaign: %v", err)
	}

	// Reopen exactly as the HTTP handler does, then drive the executor with
	// the cancelable context.
	reopened, err := db.ReopenCampaignForRetry(campaign.ID)
	if err != nil || !reopened {
		t.Fatalf("reopen staged campaign: reopened=%v err=%v", reopened, err)
	}

	stub.resetCalls()
	stub.blockCallsAfter(1) // the first retried case answers; the second blocks
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		srv.Evaluator().RetryFailedResults(ctx, campaign.ID)
	}()
	t.Cleanup(func() {
		cancel()
		stub.releaseGlobal()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Error("retry executor did not stop within 10s of cancellation")
		}
	})

	waitFor(t, "second retried call reaching the stub", func() bool {
		return stub.grandTotalCalls() >= 2
	})
	cancel()
	stub.releaseGlobal()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("retry executor did not finish after cancellation")
	}

	// The interrupted run failed (the second model's cell never started) and
	// the campaign re-settles failed; work completed before the cancel kept
	// its score, everything untouched kept its null row.
	if got := runDetail(t, ts.URL, run.ID)["status"]; got != "failed" {
		t.Errorf("run status after canceled retry = %v, want failed", got)
	}
	if got := getCampaign(t, ts.URL, campaign.ID)["status"]; got != "failed" {
		t.Errorf("campaign status after canceled retry = %v, want failed", got)
	}
	detail := runDetail(t, ts.URL, run.ID)
	smart := map[int64]interface{}{}
	for _, r := range resultsByModel(detail, "smart-model") {
		smart[int64(r["case_id"].(float64))] = r["score"]
	}
	if smart[case1] != 1.0 {
		t.Errorf("case judged before cancel: score = %v, want 1", smart[case1])
	}
	if smart[case2] != nil {
		t.Errorf("case interrupted by cancel: score = %v, want null", smart[case2])
	}
	for _, r := range resultsByModel(detail, "chat-two") {
		if r["score"] != nil {
			t.Errorf("never-started cell case %v score = %v, want null (untouched)", r["case_id"], r["score"])
		}
	}
	report := campaignReport(t, ts.URL, campaign.ID)
	if got := int(report["failed_results"].(float64)); got != 3 {
		t.Errorf("failed_results after canceled retry = %d, want 3", got)
	}

	// The interrupted retry stays retryable: a second pass heals the batch.
	resp := postRetryFailed(t, ts.URL, campaign.ID)
	resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("second retry-failed: expected 202, got %d", resp.StatusCode)
	}
	waitCampaignStatus(t, ts.URL, campaign.ID, "done")
	if got := runDetail(t, ts.URL, run.ID)["status"]; got != "done" {
		t.Errorf("run status after healing retry = %v, want done", got)
	}
	report = campaignReport(t, ts.URL, campaign.ID)
	if got := report["failed_results"].(float64); got != 0 {
		t.Errorf("failed_results after healing retry = %v, want 0", got)
	}
}
