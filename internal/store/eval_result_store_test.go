package store

import (
	"strings"
	"testing"
	"time"
)

// TestOpenEnablesWAL pins the GH #150 write-path caliber: every database
// opened through Open runs in WAL mode with NORMAL synchronous, so the
// probe/eval write streams stop paying a journal rewrite and an fsync per
// committed row.
func TestOpenEnablesWAL(t *testing.T) {
	db := openTestDB(t)

	var mode string
	if err := db.conn.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil {
		t.Fatalf("read journal_mode: %v", err)
	}
	if mode != "wal" {
		t.Errorf("journal_mode = %q, want wal", mode)
	}
	var syncMode int
	if err := db.conn.QueryRow("PRAGMA synchronous").Scan(&syncMode); err != nil {
		t.Fatalf("read synchronous: %v", err)
	}
	if syncMode != 1 { // 1 = NORMAL
		t.Errorf("synchronous = %d, want 1 (NORMAL)", syncMode)
	}
}

// TestCreateEvalResultsBatch pins the batch insert path: rows land
// identically to the single-row API (V1 profile fallback included), an
// empty batch is a no-op, and the rows read back through ListEvalResults.
func TestCreateEvalResultsBatch(t *testing.T) {
	db := openTestDB(t)

	hub, err := db.CreateHub("batch-hub", "http://batch.test", "tok-batch-0000")
	if err != nil {
		t.Fatalf("create hub: %v", err)
	}
	model, err := db.CreateModel(hub.ID, "batch-model", []string{"openai"})
	if err != nil {
		t.Fatalf("create model: %v", err)
	}
	campaign, err := db.CreateCampaign("manual", []int64{model.ID}, time.Now().UTC())
	if err != nil {
		t.Fatalf("create campaign: %v", err)
	}
	suites, err := db.ListSuites()
	if err != nil || len(suites) == 0 {
		t.Fatalf("list suites: %v (n=%d)", err, len(suites))
	}
	run, err := db.CreateEvalRun(campaign.ID, suites[0].ID, "manual", "judge-x")
	if err != nil {
		t.Fatalf("create run: %v", err)
	}

	reason := "circuit open: test"
	rows := []EvalResult{
		{EvalRunID: run.ID, ModelDBID: model.ID, ModelID: model.ModelID, CaseID: 1, VerdictDetail: &reason},
		{EvalRunID: run.ID, ModelDBID: model.ID, ModelID: model.ModelID, CaseID: 2, VerdictDetail: &reason, VerdictProfile: VerdictProfileV2},
	}
	if err := db.CreateEvalResultsBatch(rows); err != nil {
		t.Fatalf("batch insert: %v", err)
	}
	if err := db.CreateEvalResultsBatch(nil); err != nil {
		t.Fatalf("empty batch must be a no-op: %v", err)
	}

	got, err := db.ListEvalResults(run.ID)
	if err != nil {
		t.Fatalf("list results: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d results, want 2", len(got))
	}
	if got[0].VerdictProfile != VerdictProfileV1 {
		t.Errorf("row without profile got %q, want V1 fallback", got[0].VerdictProfile)
	}
	if got[1].VerdictProfile != VerdictProfileV2 {
		t.Errorf("explicit profile got %q, want %q", got[1].VerdictProfile, VerdictProfileV2)
	}
	if got[0].VerdictDetail == nil || *got[0].VerdictDetail != reason {
		t.Errorf("verdict detail = %v, want %q", got[0].VerdictDetail, reason)
	}
}

// TestListEvalRunAverageScores pins the GH #151 aggregate read: one query
// returns every run's NULL-skipping mean, and a run with only unscored
// rows is absent (nil mean) rather than averaging to zero.
func TestListEvalRunAverageScores(t *testing.T) {
	db := openTestDB(t)

	hub, err := db.CreateHub("avg-hub", "http://avg.test", "tok-avg-0000")
	if err != nil {
		t.Fatalf("create hub: %v", err)
	}
	model, err := db.CreateModel(hub.ID, "avg-model", []string{"openai"})
	if err != nil {
		t.Fatalf("create model: %v", err)
	}
	campaign, err := db.CreateCampaign("manual", []int64{model.ID}, time.Now().UTC())
	if err != nil {
		t.Fatalf("create campaign: %v", err)
	}
	suites, err := db.ListSuites()
	if err != nil || len(suites) < 2 {
		t.Fatalf("list suites: %v (n=%d)", err, len(suites))
	}
	runA, err := db.CreateEvalRun(campaign.ID, suites[0].ID, "manual", "judge-x")
	if err != nil {
		t.Fatalf("create run a: %v", err)
	}
	runB, err := db.CreateEvalRun(campaign.ID, suites[1].ID, "manual", "judge-x")
	if err != nil {
		t.Fatalf("create run b: %v", err)
	}

	// Run A: 1.0, 0.5 and one unscored row — mean over the two scored only.
	one, half := 1.0, 0.5
	for i, score := range []*float64{&one, &half, nil} {
		if _, err := db.CreateEvalResult(EvalResult{
			EvalRunID: runA.ID, ModelDBID: model.ID, ModelID: model.ModelID,
			CaseID: int64(i + 1), Score: score,
		}); err != nil {
			t.Fatalf("seed result %d: %v", i, err)
		}
	}
	// Run B: only unscored rows.
	if _, err := db.CreateEvalResult(EvalResult{
		EvalRunID: runB.ID, ModelDBID: model.ID, ModelID: model.ModelID, CaseID: 1,
	}); err != nil {
		t.Fatalf("seed null result: %v", err)
	}

	avgs, err := db.ListEvalRunAverageScores([]int64{runA.ID, runB.ID})
	if err != nil {
		t.Fatalf("aggregate averages: %v", err)
	}
	if got, ok := avgs[runA.ID]; !ok || got < 0.749 || got > 0.751 {
		t.Errorf("run A average = %v (present=%v), want 0.75 (NULL excluded)", got, ok)
	}
	if _, ok := avgs[runB.ID]; ok {
		t.Errorf("run B must be absent (no scored rows), got %v", avgs[runB.ID])
	}
}

// TestModelTrendUsesModelIndex pins the GH #150 index: the trend query's
// model_db_id filter must resolve through idx_eval_results_model instead of
// scanning the eval_results detail table.
func TestModelTrendUsesModelIndex(t *testing.T) {
	db := openTestDB(t)

	rows, err := db.conn.Query(`
		EXPLAIN QUERY PLAN
		SELECT r.campaign_id, AVG(res.score)
		FROM eval_runs r
		JOIN eval_results res ON res.eval_run_id = r.id
		WHERE res.model_db_id = 1 AND r.status = 'done'
		GROUP BY r.campaign_id
	`)
	if err != nil {
		t.Fatalf("explain: %v", err)
	}
	defer rows.Close()
	var plan strings.Builder
	for rows.Next() {
		var id, parent, notused int
		var detail string
		if err := rows.Scan(&id, &parent, &notused, &detail); err != nil {
			t.Fatalf("scan plan: %v", err)
		}
		plan.WriteString(detail)
		plan.WriteByte('\n')
	}
	if !strings.Contains(plan.String(), "idx_eval_results_model") {
		t.Errorf("trend query plan does not use idx_eval_results_model:\n%s", plan.String())
	}
}
