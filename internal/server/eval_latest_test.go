package server_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

// latestScores fetches GET /api/evals/latest.
func latestScores(t *testing.T, base string) []map[string]interface{} {
	t.Helper()
	resp := doGet(t, base+"/api/evals/latest")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/evals/latest: expected 200, got %d", resp.StatusCode)
	}
	var env envelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode latest scores: %v", err)
	}
	var out []map[string]interface{}
	if err := json.Unmarshal(env.Data, &out); err != nil {
		t.Fatalf("unmarshal latest scores: %v", err)
	}
	return out
}

// findLatest locates the latest-score row for one (suite key, model) pair.
func findLatest(t *testing.T, rows []map[string]interface{}, suiteKey, modelID string) map[string]interface{} {
	t.Helper()
	for _, row := range rows {
		if row["suite_key"] == suiteKey && row["model_id"] == modelID {
			return row
		}
	}
	t.Fatalf("no latest score for (%s, %s) in %v", suiteKey, modelID, rows)
	return nil
}

// TestEvalLatestScores runs two rounds over the same suite plus one over a
// second suite and asserts every (suite, model) pair reports the aggregate
// of its most recent done run — including pairs absent from the newest run.
func TestEvalLatestScores(t *testing.T) {
	ts, stub, _ := setupEvalEnv(t)
	smartID := createEvalModel(t, ts.URL, stub.URL, "smart-model")
	dumbID := createEvalModel(t, ts.URL, stub.URL, "dumb-model")

	// Empty before any run finishes.
	if rows := latestScores(t, ts.URL); len(rows) != 0 {
		t.Fatalf("expected no latest scores before any run, got %v", rows)
	}

	instructionID := suiteIDByKey(t, ts.URL, "cap_instruction")

	// Round 1: both models over the instruction suite.
	run1 := triggerEval(t, ts.URL, instructionID, smartID, dumbID)
	waitEvalDone(t, ts.URL, run1)

	rows := latestScores(t, ts.URL)
	if len(rows) != 2 {
		t.Fatalf("after run 1: expected 2 pairs, got %v", rows)
	}
	smart := findLatest(t, rows, "cap_instruction", "smart-model")
	if smart["score"] != 1.0 {
		t.Errorf("smart instruction score = %v, want 1", smart["score"])
	}
	if int64(smart["eval_run_id"].(float64)) != run1 {
		t.Errorf("smart instruction run = %v, want %d", smart["eval_run_id"], run1)
	}
	if int64(smart["model_db_id"].(float64)) != smartID {
		t.Errorf("smart model_db_id = %v, want %d", smart["model_db_id"], smartID)
	}
	if smart["finished_at"] == nil || smart["finished_at"] == "" {
		t.Error("finished_at should be set on a done run")
	}
	dumb := findLatest(t, rows, "cap_instruction", "dumb-model")
	if dumb["score"] != 0.0 {
		t.Errorf("dumb instruction score = %v, want 0", dumb["score"])
	}

	// Round 2: only the smart model, over the same suite. The dumb model's
	// latest row must keep pointing at round 1.
	run2 := triggerEval(t, ts.URL, instructionID, smartID)
	waitEvalDone(t, ts.URL, run2)

	rows = latestScores(t, ts.URL)
	if len(rows) != 2 {
		t.Fatalf("after run 2: expected still 2 pairs, got %v", rows)
	}
	smart = findLatest(t, rows, "cap_instruction", "smart-model")
	if int64(smart["eval_run_id"].(float64)) != run2 {
		t.Errorf("smart instruction run = %v, want %d", smart["eval_run_id"], run2)
	}
	dumb = findLatest(t, rows, "cap_instruction", "dumb-model")
	if int64(dumb["eval_run_id"].(float64)) != run1 {
		t.Errorf("dumb instruction run = %v, want %d (untouched by run 2)", dumb["eval_run_id"], run1)
	}

	// A run over a second suite adds its pairs without disturbing the first.
	reasoningID := suiteIDByKey(t, ts.URL, "cap_reasoning")
	run3 := triggerEval(t, ts.URL, reasoningID, smartID)
	waitEvalDone(t, ts.URL, run3)

	rows = latestScores(t, ts.URL)
	if len(rows) != 3 {
		t.Fatalf("after run 3: expected 3 pairs, got %v", rows)
	}
	reasoning := findLatest(t, rows, "cap_reasoning", "smart-model")
	if reasoning["score"] != 1.0 {
		t.Errorf("smart cap_reasoning score = %v, want 1", reasoning["score"])
	}
	if int64(reasoning["suite_id"].(float64)) != reasoningID {
		t.Errorf("cap_reasoning suite_id = %v, want %d", reasoning["suite_id"], reasoningID)
	}
}
