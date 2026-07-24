package server_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/taliove/hubscope/internal/server"
	"github.com/taliove/hubscope/internal/store"
)

// setupEvalEnv builds an isolated API server + stub Hub + real SQLite DB.
func setupEvalEnv(t *testing.T) (*httptest.Server, *evalStubHub, *store.DB) {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	seedTestUser(t, db)
	stub := newEvalStubHub()
	ts := httptest.NewServer(server.New(db, server.WithRateLimits(server.RateLimits{})))
	t.Cleanup(func() {
		ts.Close()
		stub.Close()
		db.Close()
	})
	return ts, stub, db
}

// createEvalModel registers a hub pointing at the stub plus one model, and
// returns the model's database ID.
func createEvalModel(t *testing.T, base, hubBase, modelID string) int64 {
	t.Helper()
	resp := doPost(t, base+"/api/hubs", map[string]interface{}{
		"name": "Eval Hub " + modelID, "base_url": hubBase, "token": "eval-token",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create hub for %s: got %d", modelID, resp.StatusCode)
	}
	var env envelope
	_ = json.NewDecoder(resp.Body).Decode(&env)
	resp.Body.Close()
	var hub map[string]interface{}
	_ = json.Unmarshal(env.Data, &hub)

	resp = doPost(t, base+"/api/models", map[string]interface{}{
		"hub_id": hub["id"], "model_id": modelID,
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create model %s: got %d", modelID, resp.StatusCode)
	}
	_ = json.NewDecoder(resp.Body).Decode(&env)
	resp.Body.Close()
	var model map[string]interface{}
	_ = json.Unmarshal(env.Data, &model)
	return int64(model["id"].(float64))
}

// suiteIDByKey resolves a seeded suite's database ID via the API.
func suiteIDByKey(t *testing.T, base, key string) int64 {
	t.Helper()
	resp := doGet(t, base+"/api/suites")
	var env envelope
	_ = json.NewDecoder(resp.Body).Decode(&env)
	resp.Body.Close()
	var suites []map[string]interface{}
	_ = json.Unmarshal(env.Data, &suites)
	for _, s := range suites {
		if s["key"] == key {
			return int64(s["id"].(float64))
		}
	}
	t.Fatalf("suite %q not found", key)
	return 0
}

// triggerEval starts a manual single-suite eval and returns its run ID;
// expects HTTP 202 with the created campaign wrapping exactly one run.
func triggerEval(t *testing.T, base string, suiteID int64, modelIDs ...int64) int64 {
	t.Helper()
	resp := doPost(t, base+"/api/evals", map[string]interface{}{
		"suite_id": suiteID, "model_ids": modelIDs,
	})
	if resp.StatusCode != http.StatusAccepted {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("POST /api/evals: expected 202, got %d: %s", resp.StatusCode, b)
	}
	var env envelope
	_ = json.NewDecoder(resp.Body).Decode(&env)
	resp.Body.Close()
	var campaign map[string]interface{}
	_ = json.Unmarshal(env.Data, &campaign)
	if campaign["trigger"] != "manual" {
		t.Errorf("new campaign trigger = %v, want manual", campaign["trigger"])
	}
	runs, ok := campaign["runs"].([]interface{})
	if !ok || len(runs) != 1 {
		t.Fatalf("manual single-suite trigger: campaign runs = %v, want exactly 1", campaign["runs"])
	}
	run := runs[0].(map[string]interface{})
	if run["status"] != "running" {
		t.Errorf("new run status = %v, want running", run["status"])
	}
	if run["trigger"] != "manual" {
		t.Errorf("new run trigger = %v, want manual", run["trigger"])
	}
	if run["judge_model"] != store.DefaultJudgeModel {
		t.Errorf("judge_model = %v, want %s", run["judge_model"], store.DefaultJudgeModel)
	}
	if int64(run["campaign_id"].(float64)) != int64(campaign["id"].(float64)) {
		t.Errorf("run campaign_id = %v, want its campaign %v", run["campaign_id"], campaign["id"])
	}
	return int64(run["id"].(float64))
}

// waitEvalDone polls a run until it leaves the running state.
func waitEvalDone(t *testing.T, base string, runID int64) map[string]interface{} {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		resp := doGet(t, fmt.Sprintf("%s/api/evals/%d", base, runID))
		var env envelope
		_ = json.NewDecoder(resp.Body).Decode(&env)
		resp.Body.Close()
		var run map[string]interface{}
		_ = json.Unmarshal(env.Data, &run)
		if run["status"] == "done" || run["status"] == "failed" {
			return run
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("eval run %d did not finish in time", runID)
	return nil
}

// resultsByModel filters a run's results down to one model.
func resultsByModel(run map[string]interface{}, modelID string) []map[string]interface{} {
	var out []map[string]interface{}
	for _, r := range run["results"].([]interface{}) {
		rm := r.(map[string]interface{})
		if rm["model_id"] == modelID {
			out = append(out, rm)
		}
	}
	return out
}

// TestEvalRuleVerdicts runs the basic suite (contains + regex modes) against
// a smart and a dumb model and asserts per-case scores plus the aggregate.
func TestEvalRuleVerdicts(t *testing.T) {
	ts, stub, _ := setupEvalEnv(t)
	smartID := createEvalModel(t, ts.URL, stub.URL, "smart-model")
	dumbID := createEvalModel(t, ts.URL, stub.URL, "dumb-model")
	suiteID := suiteIDByKey(t, ts.URL, "basic")

	runID := triggerEval(t, ts.URL, suiteID, smartID, dumbID)
	run := waitEvalDone(t, ts.URL, runID)

	if run["status"] != "done" {
		t.Fatalf("run status = %v, want done", run["status"])
	}
	if run["finished_at"] == nil {
		t.Error("finished_at should be set once done")
	}

	smart := resultsByModel(run, "smart-model")
	if len(smart) != 12 {
		t.Fatalf("smart model has %d results, want 12", len(smart))
	}
	for _, r := range smart {
		if r["score"] != 1.0 {
			t.Errorf("smart case %v score = %v, want 1 (detail: %v)", r["case_id"], r["score"], r["verdict_detail"])
		}
		if r["answer_text"] == nil || r["answer_text"] == "" {
			t.Errorf("smart case %v missing answer_text", r["case_id"])
		}
		if r["input_tokens"] == nil || r["output_tokens"] == nil {
			t.Errorf("smart case %v missing token usage", r["case_id"])
		}
		if !strings.Contains(r["verdict_detail"].(string), "matched") {
			t.Errorf("smart verdict_detail should explain the match: %v", r["verdict_detail"])
		}
	}

	dumb := resultsByModel(run, "dumb-model")
	if len(dumb) != 12 {
		t.Fatalf("dumb model has %d results, want 12", len(dumb))
	}
	for _, r := range dumb {
		if r["score"] != 0.0 {
			t.Errorf("dumb case %v score = %v, want 0 (detail: %v)", r["case_id"], r["score"], r["verdict_detail"])
		}
	}

	// Aggregate: (12 x 1 + 12 x 0) / 24 = 0.5.
	if score, ok := run["score"].(float64); !ok || score != 0.5 {
		t.Errorf("run score = %v, want 0.5", run["score"])
	}

	// The smart model must have been called over anthropic (preferred).
	if !stub.sawProtocol("smart-model", "anthropic") {
		t.Error("expected smart-model to be called over anthropic")
	}

	// The run must also appear in the list endpoint.
	listResp := doGet(t, ts.URL+"/api/evals")
	var env envelope
	_ = json.NewDecoder(listResp.Body).Decode(&env)
	listResp.Body.Close()
	var runs []map[string]interface{}
	_ = json.Unmarshal(env.Data, &runs)
	if len(runs) != 1 || int64(runs[0]["id"].(float64)) != runID {
		t.Errorf("GET /api/evals = %v, want the single run %d", runs, runID)
	}
	if runs[0]["score"] != 0.5 {
		t.Errorf("list endpoint run score = %v, want 0.5", runs[0]["score"])
	}
}
