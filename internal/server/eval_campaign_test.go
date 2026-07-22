package server_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/taliove2009/hubscope/internal/scheduler"
	"github.com/taliove2009/hubscope/internal/server"
	"github.com/taliove2009/hubscope/internal/store"
)

// getCampaign fetches GET /api/campaigns/{id} and returns the decoded detail.
func getCampaign(t *testing.T, base string, id int64) map[string]interface{} {
	t.Helper()
	resp := doGet(t, fmt.Sprintf("%s/api/campaigns/%d", base, id))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("GET /api/campaigns/%d: expected 200, got %d: %s", id, resp.StatusCode, b)
	}
	var env envelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode campaign: %v", err)
	}
	var campaign map[string]interface{}
	if err := json.Unmarshal(env.Data, &campaign); err != nil {
		t.Fatalf("unmarshal campaign: %v", err)
	}
	return campaign
}

// listCampaigns fetches GET /api/campaigns.
func listCampaigns(t *testing.T, base string) []map[string]interface{} {
	t.Helper()
	resp := doGet(t, base+"/api/campaigns")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/campaigns: expected 200, got %d", resp.StatusCode)
	}
	var env envelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode campaigns: %v", err)
	}
	var campaigns []map[string]interface{}
	if err := json.Unmarshal(env.Data, &campaigns); err != nil {
		t.Fatalf("unmarshal campaigns: %v", err)
	}
	return campaigns
}

// campaignRuns extracts the runs array of a campaign detail.
func campaignRuns(t *testing.T, campaign map[string]interface{}) []map[string]interface{} {
	t.Helper()
	raw, ok := campaign["runs"].([]interface{})
	if !ok {
		t.Fatalf("campaign runs missing or wrong type: %v", campaign)
	}
	runs := make([]map[string]interface{}, 0, len(raw))
	for _, r := range raw {
		runs = append(runs, r.(map[string]interface{}))
	}
	return runs
}

// campaignProgress extracts the progress object of a campaign DTO.
func campaignProgress(t *testing.T, campaign map[string]interface{}) map[string]interface{} {
	t.Helper()
	p, ok := campaign["progress"].(map[string]interface{})
	if !ok {
		t.Fatalf("campaign progress missing or wrong type: %v", campaign)
	}
	return p
}

// waitCampaignStatus polls a campaign until it reaches one of the wanted
// statuses, then returns the final detail.
func waitCampaignStatus(t *testing.T, base string, id int64, want ...string) map[string]interface{} {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		campaign := getCampaign(t, base, id)
		for _, w := range want {
			if campaign["status"] == w {
				return campaign
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("campaign %d did not reach %v in time", id, want)
	return nil
}

// triggerFullSweep posts POST /api/evals without a suite_id and returns the
// created campaign as decoded from the 202 response.
func triggerFullSweep(t *testing.T, base string) map[string]interface{} {
	t.Helper()
	resp := doPost(t, base+"/api/evals", map[string]interface{}{})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("POST /api/evals (full sweep): expected 202, got %d: %s", resp.StatusCode, b)
	}
	var env envelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode sweep campaign: %v", err)
	}
	var campaign map[string]interface{}
	if err := json.Unmarshal(env.Data, &campaign); err != nil {
		t.Fatalf("unmarshal sweep campaign: %v", err)
	}
	if campaign["trigger"] != "manual" {
		t.Errorf("sweep campaign trigger = %v, want manual", campaign["trigger"])
	}
	return campaign
}

// suiteCount returns how many suites exist, read through the API so
// expectations track the seed bank instead of hardcoding a count.
func suiteCount(t *testing.T, base string) int {
	t.Helper()
	resp := doGet(t, base+"/api/suites")
	defer resp.Body.Close()
	var env envelope
	_ = json.NewDecoder(resp.Body).Decode(&env)
	var suites []map[string]interface{}
	_ = json.Unmarshal(env.Data, &suites)
	return len(suites)
}

// runModelIDs returns the distinct model_id strings covered by a run's results.
func runModelIDs(t *testing.T, base string, runID int64) map[string]bool {
	t.Helper()
	resp := doGet(t, fmt.Sprintf("%s/api/evals/%d", base, runID))
	defer resp.Body.Close()
	var env envelope
	_ = json.NewDecoder(resp.Body).Decode(&env)
	var detail map[string]interface{}
	_ = json.Unmarshal(env.Data, &detail)
	models := map[string]bool{}
	for _, raw := range detail["results"].([]interface{}) {
		models[raw.(map[string]interface{})["model_id"].(string)] = true
	}
	return models
}

// TestFullSweepCampaign covers the one-click full evaluation: POST /api/evals
// without a suite_id produces one campaign with one run per suite, every run
// covering all active chat models — and never the non_chat one.
func TestFullSweepCampaign(t *testing.T) {
	ts, stub, db := setupEvalEnv(t)
	createEvalModel(t, ts.URL, stub.URL, "smart-model")
	createEvalModel(t, ts.URL, stub.URL, "chat-two")
	nonChatID := createEvalModel(t, ts.URL, stub.URL, "embed-model")
	if err := db.SetModelCapability(nonChatID, "non_chat"); err != nil {
		t.Fatalf("tag non_chat model: %v", err)
	}

	campaign := triggerFullSweep(t, ts.URL)
	campaignID := int64(campaign["id"].(float64))
	final := waitCampaignStatus(t, ts.URL, campaignID, "done", "failed")

	if final["status"] != "done" {
		t.Fatalf("sweep campaign status = %v, want done", final["status"])
	}
	if final["finished_at"] == nil {
		t.Error("done campaign must carry finished_at")
	}
	if final["started_at"] == nil || final["created_at"] == nil {
		t.Errorf("campaign must carry started_at and created_at: %v", final)
	}

	// One run per suite, all under this campaign.
	suites := suiteCount(t, ts.URL)
	runs := campaignRuns(t, final)
	if len(runs) != suites {
		t.Fatalf("campaign has %d runs, want one per suite (%d)", len(runs), suites)
	}
	seenSuites := map[int64]bool{}
	for _, run := range runs {
		if int64(run["campaign_id"].(float64)) != campaignID {
			t.Errorf("run %v campaign_id = %v, want %d", run["id"], run["campaign_id"], campaignID)
		}
		if run["trigger"] != "manual" {
			t.Errorf("run %v trigger = %v, want manual", run["id"], run["trigger"])
		}
		if run["status"] != "done" {
			t.Errorf("run %v status = %v, want done", run["id"], run["status"])
		}
		suiteID := int64(run["suite_id"].(float64))
		if seenSuites[suiteID] {
			t.Errorf("suite %d has more than one run in the campaign", suiteID)
		}
		seenSuites[suiteID] = true
	}

	// Every run covered exactly the two chat models; the non_chat model was
	// excluded everywhere.
	for _, run := range runs {
		models := runModelIDs(t, ts.URL, int64(run["id"].(float64)))
		if len(models) != 2 || !models["smart-model"] || !models["chat-two"] {
			t.Errorf("run %v covered models %v, want {smart-model, chat-two}", run["id"], models)
		}
		if models["embed-model"] {
			t.Errorf("run %v covered the non_chat model", run["id"])
		}
	}

	// Aggregate progress on the list endpoint matches the settled campaign.
	progress := campaignProgress(t, final)
	if int(progress["total"].(float64)) != suites || int(progress["done"].(float64)) != suites {
		t.Errorf("campaign progress = %v, want total=done=%d", progress, suites)
	}
	if int(progress["failed"].(float64)) != 0 || int(progress["running"].(float64)) != 0 {
		t.Errorf("campaign progress = %v, want failed=running=0", progress)
	}
	var listed map[string]interface{}
	for _, c := range listCampaigns(t, ts.URL) {
		if int64(c["id"].(float64)) == campaignID {
			listed = c
		}
	}
	if listed == nil {
		t.Fatalf("campaign %d missing from GET /api/campaigns", campaignID)
	}
	lp := campaignProgress(t, listed)
	if int(lp["total"].(float64)) != suites || int(lp["done"].(float64)) != suites {
		t.Errorf("listed campaign progress = %v, want total=done=%d", lp, suites)
	}

	// The runs list endpoint groups every run by the same campaign_id.
	for _, r := range listEvalRuns(t, ts.URL) {
		if int64(r["campaign_id"].(float64)) != campaignID {
			t.Errorf("list endpoint run %v campaign_id = %v, want %d", r["id"], r["campaign_id"], campaignID)
		}
	}
}

// TestManualSingleSuiteCampaign verifies the single-suite manual trigger also
// lands in a campaign — with exactly one run — keeping the data model unified.
func TestManualSingleSuiteCampaign(t *testing.T) {
	ts, stub, _ := setupEvalEnv(t)
	smartID := createEvalModel(t, ts.URL, stub.URL, "smart-model")
	suiteID := suiteIDByKey(t, ts.URL, "basic")

	runID := triggerEval(t, ts.URL, suiteID, smartID)
	waitEvalDone(t, ts.URL, runID)

	// The run's campaign settles to done with exactly this one run attached.
	resp := doGet(t, fmt.Sprintf("%s/api/evals/%d", ts.URL, runID))
	var env envelope
	_ = json.NewDecoder(resp.Body).Decode(&env)
	resp.Body.Close()
	var run map[string]interface{}
	_ = json.Unmarshal(env.Data, &run)
	campaignID, ok := run["campaign_id"].(float64)
	if !ok || campaignID == 0 {
		t.Fatalf("run detail campaign_id missing: %v", run)
	}

	campaign := waitCampaignStatus(t, ts.URL, int64(campaignID), "done", "failed")
	if campaign["status"] != "done" {
		t.Fatalf("single-run campaign status = %v, want done", campaign["status"])
	}
	runs := campaignRuns(t, campaign)
	if len(runs) != 1 || int64(runs[0]["id"].(float64)) != runID {
		t.Errorf("campaign runs = %v, want exactly run %d", runs, runID)
	}
	progress := campaignProgress(t, campaign)
	if int(progress["total"].(float64)) != 1 || int(progress["done"].(float64)) != 1 {
		t.Errorf("campaign progress = %v, want total=done=1", progress)
	}
}

// TestFullSweepRequiresChatModel rejects a full sweep when no active
// chat-capable model exists.
func TestFullSweepRequiresChatModel(t *testing.T) {
	ts, stub, db := setupEvalEnv(t)
	nonChatID := createEvalModel(t, ts.URL, stub.URL, "embed-model")
	if err := db.SetModelCapability(nonChatID, "non_chat"); err != nil {
		t.Fatalf("tag non_chat model: %v", err)
	}

	resp := doPost(t, ts.URL+"/api/evals", map[string]interface{}{})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("full sweep without chat models: expected 400, got %d", resp.StatusCode)
	}
	if got := len(listCampaigns(t, ts.URL)); got != 0 {
		t.Errorf("rejected sweep must not create a campaign, found %d", got)
	}
}

// TestWeeklyBatchProducesOneCampaign advances the fake clock into the Sunday
// window and asserts the weekly batch is one campaign — not a loose run
// group — exactly once per week.
func TestWeeklyBatchProducesOneCampaign(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "weekly-campaign.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	stub := newEvalStubHub()
	t.Cleanup(stub.Close)
	srv := server.New(db, testAdminPassword, server.WithRateLimits(server.RateLimits{}))
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	createEvalModel(t, ts.URL, stub.URL, "smart-model")
	suites := suiteCount(t, ts.URL)

	clock := scheduler.NewFakeClock(time.Date(2026, 7, 18, 23, 30, 0, 0, time.UTC)) // Saturday
	startEvalWorker(t, db, srv, clock)
	if got := len(listCampaigns(t, ts.URL)); got != 0 {
		t.Fatalf("before the window: expected 0 campaigns, got %d", got)
	}

	// Sunday 01:30: the weekly batch fires as a single scheduled campaign.
	clock.Advance(2 * time.Hour)
	waitFor(t, "one scheduled campaign", func() bool {
		campaigns := listCampaigns(t, ts.URL)
		return len(campaigns) == 1 && campaigns[0]["trigger"] == "scheduled"
	})
	campaigns := listCampaigns(t, ts.URL)
	campaignID := int64(campaigns[0]["id"].(float64))
	final := waitCampaignStatus(t, ts.URL, campaignID, "done", "failed")
	if final["status"] != "done" {
		t.Fatalf("weekly campaign status = %v, want done", final["status"])
	}
	if got := len(campaignRuns(t, final)); got != suites {
		t.Errorf("weekly campaign has %d runs, want one per suite (%d)", got, suites)
	}

	// Later the same Sunday: no second campaign.
	clock.Advance(3 * time.Hour)
	time.Sleep(200 * time.Millisecond)
	if got := len(listCampaigns(t, ts.URL)); got != 1 {
		t.Fatalf("same Sunday: expected still 1 campaign, got %d", got)
	}

	// Next Sunday: a fresh campaign.
	clock.Advance(164 * time.Hour)
	waitFor(t, "next week's campaign", func() bool {
		return len(listCampaigns(t, ts.URL)) == 2
	})
}

// TestCampaignStatusWhileRunning freezes the hub mid-sweep and asserts the
// campaign reports running with live run progress, then settles to done once
// released.
func TestCampaignStatusWhileRunning(t *testing.T) {
	ts, stub, _ := setupEvalEnv(t)
	createEvalModel(t, ts.URL, stub.URL, "smart-model")
	stub.resetCalls()
	stub.blockCalls()
	t.Cleanup(stub.release)

	campaign := triggerFullSweep(t, ts.URL)
	campaignID := int64(campaign["id"].(float64))

	waitFor(t, "first sweep call reaching the stub", func() bool {
		return stub.sawModel("smart-model")
	})
	mid := getCampaign(t, ts.URL, campaignID)
	if mid["status"] != "running" {
		t.Errorf("mid-sweep campaign status = %v, want running", mid["status"])
	}
	progress := campaignProgress(t, mid)
	if int(progress["running"].(float64)) < 1 {
		t.Errorf("mid-sweep progress = %v, want at least one running run", progress)
	}

	stub.release()
	final := waitCampaignStatus(t, ts.URL, campaignID, "done")
	if final["finished_at"] == nil {
		t.Error("settled campaign must carry finished_at")
	}
}

// TestCampaignFailedWhenBatchAborted cancels the weekly worker mid-run and
// asserts the campaign settles to failed once its run fails.
func TestCampaignFailedWhenBatchAborted(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "aborted-campaign.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	stub := newEvalStubHub()
	t.Cleanup(stub.Close)
	srv := server.New(db, testAdminPassword, server.WithRateLimits(server.RateLimits{}))
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	createEvalModel(t, ts.URL, stub.URL, "smart-model")
	// A second chat model: the run blocks inside the first model's cases, so
	// cancelling before release makes the second model's iteration observe
	// the canceled context and fail the run deterministically.
	createEvalModel(t, ts.URL, stub.URL, "chat-two")
	stub.resetCalls()
	stub.blockCalls()

	clock := scheduler.NewFakeClock(time.Date(2026, 7, 19, 1, 30, 0, 0, time.UTC)) // a Sunday
	worker := scheduler.NewEvalWorker(db, srv.Evaluator(), clock,
		scheduler.WithEvalPollInterval(time.Minute))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		worker.Run(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		stub.release()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Error("eval worker did not stop within 10s of cancellation")
		}
	})

	waitFor(t, "first eval call reaching the stub", func() bool {
		return stub.sawModel("smart-model")
	})
	cancel()
	stub.release()

	waitFor(t, "campaign settling to failed", func() bool {
		campaigns := listCampaigns(t, ts.URL)
		return len(campaigns) == 1 && campaigns[0]["status"] == "failed"
	})
	campaign := listCampaigns(t, ts.URL)[0]
	if campaign["trigger"] != "scheduled" {
		t.Errorf("aborted campaign trigger = %v, want scheduled", campaign["trigger"])
	}
	if campaign["finished_at"] == nil {
		t.Error("failed campaign must carry finished_at")
	}
	progress := campaignProgress(t, campaign)
	if int(progress["failed"].(float64)) < 1 {
		t.Errorf("failed campaign progress = %v, want at least one failed run", progress)
	}
}

// stagePreCampaignDatabase writes a pre-ticket-29 database: the current
// schema minus the campaigns table and the eval_runs.campaign_id column,
// with two historical runs (a done manual one and a failed scheduled one).
func stagePreCampaignDatabase(t *testing.T, path string) {
	t.Helper()
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open raw db: %v", err)
	}
	defer conn.Close()

	ddl := `
		CREATE TABLE suites (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			key TEXT NOT NULL UNIQUE,
			name TEXT NOT NULL,
			version INTEGER NOT NULL DEFAULT 1
		);
		CREATE TABLE eval_runs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			suite_id INTEGER NOT NULL,
			suite_version INTEGER NOT NULL DEFAULT 1,
			"trigger" TEXT NOT NULL,
			judge_model TEXT NOT NULL,
			status TEXT NOT NULL,
			started_at TEXT NOT NULL,
			finished_at TEXT
		);
		CREATE TABLE settings (key TEXT PRIMARY KEY, value TEXT NOT NULL);
	`
	if _, err := conn.Exec(ddl); err != nil {
		t.Fatalf("create old schema: %v", err)
	}
	if _, err := conn.Exec("INSERT INTO suites (key, name) VALUES ('basic', '基础能力')"); err != nil {
		t.Fatalf("seed suite: %v", err)
	}
	runs := []struct {
		trigger, status, startedAt string
		finishedAt                 interface{}
	}{
		{"manual", "done", "2026-07-12T01:00:00Z", "2026-07-12T01:05:00Z"},
		{"scheduled", "failed", "2026-07-19T01:00:00Z", "2026-07-19T01:02:00Z"},
	}
	for _, r := range runs {
		if _, err := conn.Exec(`
			INSERT INTO eval_runs (suite_id, suite_version, "trigger", judge_model, status, started_at, finished_at)
			VALUES (1, 1, ?, 'fake-judge', ?, ?, ?)
		`, r.trigger, r.status, r.startedAt, r.finishedAt); err != nil {
			t.Fatalf("insert old run: %v", err)
		}
	}
}

// TestCampaignMigrationBackfillsOldRuns opens a pre-campaign database and
// asserts every historical run is wrapped in its own single-run migration
// campaign — trigger and status preserved — and that the migration is
// idempotent across restarts.
func TestCampaignMigrationBackfillsOldRuns(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "pre-campaign.db")
	stagePreCampaignDatabase(t, dbPath)

	serve := func() (*httptest.Server, *store.DB) {
		db, err := store.Open(dbPath)
		if err != nil {
			t.Fatalf("open migrated db: %v", err)
		}
		ts := httptest.NewServer(server.New(db, testAdminPassword, server.WithRateLimits(server.RateLimits{})))
		t.Cleanup(ts.Close)
		return ts, db
	}

	ts, db := serve()
	campaigns := listCampaigns(t, ts.URL)
	if len(campaigns) != 2 {
		t.Fatalf("after migration: expected 2 campaigns, got %d: %v", len(campaigns), campaigns)
	}

	byTrigger := map[string]map[string]interface{}{}
	for _, c := range campaigns {
		runs := campaignRuns(t, getCampaign(t, ts.URL, int64(c["id"].(float64))))
		if len(runs) != 1 {
			t.Errorf("migration campaign %v has %d runs, want exactly 1", c["id"], len(runs))
			continue
		}
		run := runs[0]
		if int64(run["campaign_id"].(float64)) != int64(c["id"].(float64)) {
			t.Errorf("run %v campaign_id = %v, want %v", run["id"], run["campaign_id"], c["id"])
		}
		if c["trigger"] != run["trigger"] {
			t.Errorf("campaign trigger = %v, want its run's %v", c["trigger"], run["trigger"])
		}
		if c["status"] != run["status"] {
			t.Errorf("campaign status = %v, want its run's %v", c["status"], run["status"])
		}
		if c["finished_at"] == nil {
			t.Errorf("terminal migration campaign must carry finished_at: %v", c)
		}
		byTrigger[c["trigger"].(string)] = c
	}
	if byTrigger["manual"] == nil || byTrigger["scheduled"] == nil {
		t.Errorf("expected one manual and one scheduled migration campaign, got %v", campaigns)
	}
	if byTrigger["manual"] != nil && byTrigger["manual"]["status"] != "done" {
		t.Errorf("manual campaign status = %v, want done", byTrigger["manual"]["status"])
	}
	if byTrigger["scheduled"] != nil && byTrigger["scheduled"]["status"] != "failed" {
		t.Errorf("scheduled campaign status = %v, want failed", byTrigger["scheduled"]["status"])
	}

	// Every run on the list endpoint now carries a campaign_id.
	for _, r := range listEvalRuns(t, ts.URL) {
		if id, ok := r["campaign_id"].(float64); !ok || id == 0 {
			t.Errorf("migrated run %v has no campaign_id: %v", r["id"], r)
		}
	}
	db.Close()

	// Re-opening must be a no-op: no duplicate migration campaigns.
	ts2, db2 := serve()
	defer db2.Close()
	if got := len(listCampaigns(t, ts2.URL)); got != 2 {
		t.Errorf("after reopen: expected still 2 campaigns, got %d", got)
	}
}
