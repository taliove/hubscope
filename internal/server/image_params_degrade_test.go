package server_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
)

// TestImageParamRuleReadFailureDegradesToMinimalBody covers GH #44: when the
// image_param_rules table cannot be read (a row with corrupt params JSON,
// e.g. a hand-edited database), the probe call sites must log and degrade to
// the minimal request body {model, prompt, n:1} instead of failing. All three
// degradation branches — server.models trialProtocols, prober.imageParamsFor,
// discovery.Syncer.imageParamsFor — resolve parameters through the single
// entry store.ImageParamsFor, so one corrupted row exercises the same error
// path at every call site; this test walks all three branches anyway. The
// corrupt row is injected via the store.ExecRawForTest test seam: data
// preparation only, with every assertion made on real HTTP behavior (W1).
func TestImageParamRuleReadFailureDegradesToMinimalBody(t *testing.T) {
	db := openTempDB(t)
	ts := newTestAPIServer(t, db)

	stub := newDiscoveryStubHub(t, []string{"gpt-image-2"})
	stub.setImageMode("gpt-image-2", "success")
	stub.setEditMode("gpt-image-2", "success")
	stub.setImageMode("gpt-image-manual", "success")

	// Sanity: with intact rules the seeded rule applies (quality=low), so the
	// post-corruption minimal body is attributable to the read failure alone.
	hubID := createHubViaAPI(t, ts.URL, stub.URL)
	waitForHubSyncStatus(t, ts.URL, hubID, "succeeded")
	if raw := lastGenerationRaw(t, stub, "gpt-image-2"); raw["quality"] != "low" {
		t.Fatalf("pre-corruption sanity: expected quality=low, body %v", raw)
	}

	// Corrupt the seeded rule row: its params JSON no longer parses, so every
	// store.ImageParamsFor call fails from here on.
	if err := db.ExecRawForTest(
		"UPDATE image_param_rules SET params = ? WHERE keyword = ?",
		"{corrupt-json", "gpt-image",
	); err != nil {
		t.Fatalf("corrupt rule row: %v", err)
	}

	// Branch 1 (server.models trialProtocols): manual model creation still
	// trials the image protocols successfully and sends the minimal body.
	resp := doPost(t, ts.URL+"/api/models", map[string]interface{}{
		"hub_id":   hubID,
		"model_id": "gpt-image-manual",
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create model with corrupt rules: expected 201, got %d", resp.StatusCode)
	}
	if raw := lastGenerationRaw(t, stub, "gpt-image-manual"); len(raw) != 3 || raw["model"] != "gpt-image-manual" {
		t.Errorf("manual trial with corrupt rules: expected minimal body {model,prompt,n}, got %v", raw)
	}

	// Branch 2 (prober.imageParamsFor): the probe round still succeeds and
	// sends the minimal body.
	models := listModelsViaAPI(t, ts.URL)
	genID := int(endpointByProtocol(t, models["gpt-image-2"], "images_generation")["id"].(float64))
	resp = doPost(t, fmt.Sprintf("%s/api/endpoints/%d/probe", ts.URL, genID), nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("probe with corrupt rules: expected 200, got %d", resp.StatusCode)
	}
	var env envelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode probe round: %v", err)
	}
	var round struct {
		Results []map[string]interface{} `json:"results"`
	}
	if err := json.Unmarshal(env.Data, &round); err != nil {
		t.Fatalf("unmarshal probe round: %v", err)
	}
	if len(round.Results) != 1 || round.Results[0]["ok"] != true {
		t.Errorf("probe round with corrupt rules: expected one ok result, got %v", round.Results)
	}
	if raw := lastGenerationRaw(t, stub, "gpt-image-2"); len(raw) != 3 || raw["model"] != "gpt-image-2" {
		t.Errorf("probe round with corrupt rules: expected minimal body {model,prompt,n}, got %v", raw)
	}

	// Branch 3 (discovery.Syncer.imageParamsFor): a fresh hub sync still
	// discovers the model and creates its image endpoints on minimal-body
	// trials.
	stub2 := newDiscoveryStubHub(t, []string{"gpt-image-7"})
	stub2.setImageMode("gpt-image-7", "success")
	stub2.setEditMode("gpt-image-7", "success")
	hubID2 := createHubViaAPI(t, ts.URL, stub2.URL)
	waitForHubSyncStatus(t, ts.URL, hubID2, "succeeded")
	models = listModelsViaAPI(t, ts.URL)
	model, ok := models["gpt-image-7"]
	if !ok {
		t.Fatal("discovery with corrupt rules: gpt-image-7 not discovered")
	}
	gen := endpointByProtocol(t, model, "images_generation")
	if gen["enabled"] != true {
		t.Errorf("discovery with corrupt rules: images_generation endpoint not enabled: %v", gen)
	}
	if raw := lastGenerationRaw(t, stub2, "gpt-image-7"); len(raw) != 3 || raw["model"] != "gpt-image-7" {
		t.Errorf("discovery trial with corrupt rules: expected minimal body {model,prompt,n}, got %v", raw)
	}
}
