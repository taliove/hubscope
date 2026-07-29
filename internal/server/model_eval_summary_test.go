package server_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
)

// TestGetModelEvalSummary verifies GET /api/models/{id}/eval-summary returns
// the model's latest campaign evaluation summary, or null when the model has
// never been evaluated.
func TestGetModelEvalSummary(t *testing.T) {
	ts, stub, _ := setupEvalEnv(t)

	// Create a model
	modelID := createEvalModel(t, ts.URL, stub.URL, "test-model")

	// Model with no evaluation history returns null
	summary := getModelEvalSummary(t, ts.URL, modelID, http.StatusOK)
	if summary != nil {
		t.Fatalf("expected null for unevaluated model, got %v", summary)
	}

	// Trigger an evaluation using the seeded cap_instruction suite
	suiteID := suiteIDByKey(t, ts.URL, "cap_instruction")
	runID := triggerEval(t, ts.URL, suiteID, modelID)
	run := waitEvalDone(t, ts.URL, runID)
	campaignID := int64(run["campaign_id"].(float64))

	// Now the model should have an evaluation summary
	summary = getModelEvalSummary(t, ts.URL, modelID, http.StatusOK)
	if summary == nil {
		t.Fatal("expected summary after campaign, got null")
	}

	// Verify structure
	summaryMap := summary.(map[string]interface{})
	if summaryMap["model_id"] != float64(modelID) {
		t.Errorf("expected model_id=%d, got %v", modelID, summaryMap["model_id"])
	}
	if summaryMap["model_id_str"] != "test-model" {
		t.Errorf("expected model_id_str=test-model, got %v", summaryMap["model_id_str"])
	}
	if summaryMap["campaign_id"] != float64(campaignID) {
		t.Errorf("expected campaign_id=%d, got %v", campaignID, summaryMap["campaign_id"])
	}
	if summaryMap["campaign_created_at"] == nil {
		t.Error("expected campaign_created_at, got nil")
	}

	// Check suite_scores array exists
	suiteScores, ok := summaryMap["suite_scores"].([]interface{})
	if !ok {
		t.Fatalf("expected suite_scores array, got %v", summaryMap["suite_scores"])
	}
	if len(suiteScores) == 0 {
		t.Error("expected at least one suite score")
	}

	// Verify suite score structure
	firstSuite := suiteScores[0].(map[string]interface{})
	if firstSuite["suite_id"] == nil {
		t.Error("expected suite_id in suite score")
	}
	if firstSuite["suite_name"] == nil {
		t.Error("expected suite_name in suite score")
	}
	if firstSuite["version"] == nil {
		t.Error("expected version in suite score")
	}

	// Total score may be null if not all cases were judged
	// but should exist as a field
	if _, exists := summaryMap["total_score"]; !exists {
		t.Error("expected total_score field (even if null)")
	}
}

// TestGetModelEvalSummaryNotFound verifies GET /api/models/{id}/eval-summary
// returns 404 for a nonexistent model.
func TestGetModelEvalSummaryNotFound(t *testing.T) {
	ts, _, _ := setupEvalEnv(t)

	resp := doGet(t, fmt.Sprintf("%s/api/models/99999/eval-summary", ts.URL))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 for nonexistent model, got %d", resp.StatusCode)
	}
}

// getModelEvalSummary fetches GET /api/models/{id}/eval-summary and asserts
// the expected status code. Returns the data payload (may be null).
func getModelEvalSummary(t *testing.T, base string, id int64, expectedStatus int) interface{} {
	t.Helper()
	url := fmt.Sprintf("%s/api/models/%d/eval-summary", base, id)
	resp := doGet(t, url)
	defer resp.Body.Close()
	if resp.StatusCode != expectedStatus {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("GET /api/models/%d/eval-summary: expected %d, got %d: %s",
			id, expectedStatus, resp.StatusCode, b)
	}
	var env envelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode eval summary: %v", err)
	}
	if env.Data == nil {
		return nil
	}
	var data interface{}
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("unmarshal eval summary data: %v", err)
	}
	return data
}
