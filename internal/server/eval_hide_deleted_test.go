package server_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
)

// TestEvalLatestHidesDeletedModels runs an eval over two models, deletes one
// of them, and asserts the comparison feed (GET /api/evals/latest) drops the
// deleted model while the historical run detail (GET /api/evals/{id}) stays
// readable: the deleted model's rows keep their model_id text and carry a
// model_deleted flag the UI turns into a badge.
func TestEvalLatestHidesDeletedModels(t *testing.T) {
	ts, stub, _ := setupEvalEnv(t)
	keepID := createEvalModel(t, ts.URL, stub.URL, "keep-model")
	dropID := createEvalModel(t, ts.URL, stub.URL, "drop-model")
	instructionID := suiteIDByKey(t, ts.URL, "cap_instruction")

	runID := triggerEval(t, ts.URL, instructionID, keepID, dropID)
	waitEvalDone(t, ts.URL, runID)

	// Baseline: both models show up in the comparison feed.
	rows := latestScores(t, ts.URL)
	if len(rows) != 2 {
		t.Fatalf("before delete: expected 2 latest rows, got %v", rows)
	}
	findLatest(t, rows, "cap_instruction", "keep-model")
	findLatest(t, rows, "cap_instruction", "drop-model")

	resp := doDelete(t, fmt.Sprintf("%s/api/models/%d", ts.URL, dropID))
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete model: expected 204, got %d", resp.StatusCode)
	}

	// The comparison feed hides the deleted model; the live one is untouched.
	rows = latestScores(t, ts.URL)
	if len(rows) != 1 {
		t.Fatalf("after delete: expected 1 latest row, got %v", rows)
	}
	keep := findLatest(t, rows, "cap_instruction", "keep-model")
	if keep["score"] != 1.0 {
		t.Errorf("keep-model score = %v, want 1", keep["score"])
	}
	for _, row := range rows {
		if row["model_id"] == "drop-model" {
			t.Errorf("deleted model must not appear in latest scores: %v", row)
		}
	}

	// The historical run detail stays readable: both models' rows render with
	// their model_id text, and only the deleted model is flagged.
	detailResp := doGet(t, fmt.Sprintf("%s/api/evals/%d", ts.URL, runID))
	defer detailResp.Body.Close()
	if detailResp.StatusCode != http.StatusOK {
		t.Fatalf("run detail after delete: expected 200, got %d", detailResp.StatusCode)
	}
	var env envelope
	if err := json.NewDecoder(detailResp.Body).Decode(&env); err != nil {
		t.Fatalf("decode run detail: %v", err)
	}
	var run map[string]interface{}
	if err := json.Unmarshal(env.Data, &run); err != nil {
		t.Fatalf("unmarshal run detail: %v", err)
	}

	dropped := resultsByModel(run, "drop-model")
	if len(dropped) == 0 {
		t.Fatal("run detail should still contain the deleted model's results")
	}
	for _, r := range dropped {
		if r["model_id"] != "drop-model" {
			t.Errorf("deleted model row model_id = %v, want drop-model", r["model_id"])
		}
		if r["model_deleted"] != true {
			t.Errorf("deleted model row model_deleted = %v, want true", r["model_deleted"])
		}
	}

	kept := resultsByModel(run, "keep-model")
	if len(kept) == 0 {
		t.Fatal("run detail should still contain the live model's results")
	}
	for _, r := range kept {
		if r["model_deleted"] != false {
			t.Errorf("live model row model_deleted = %v, want false", r["model_deleted"])
		}
	}
}
