package server_test

import (
	"fmt"
	"net/http"
	"testing"
)

// lastGenerationRaw returns the raw JSON body of the model's most recent
// /v1/images/generations call seen by the stub.
func lastGenerationRaw(t *testing.T, stub *discoveryStubHub, modelID string) map[string]interface{} {
	t.Helper()
	reqs := stub.imageRequests(modelID)
	if len(reqs) == 0 {
		t.Fatalf("stub saw no images_generation call for %s", modelID)
	}
	return reqs[len(reqs)-1].Raw
}

// lastEditFields returns the flattened multipart fields of the model's most
// recent /v1/images/edits call seen by the stub.
func lastEditFields(t *testing.T, stub *discoveryStubHub, modelID string) map[string]string {
	t.Helper()
	reqs := stub.editRequests(modelID)
	if len(reqs) == 0 {
		t.Fatalf("stub saw no images_edit call for %s", modelID)
	}
	return reqs[len(reqs)-1].Fields
}

// probeImageEndpoint fires one manual probe round for the endpoint and
// expects HTTP 200.
func probeImageEndpoint(t *testing.T, baseURL string, endpointID int) {
	t.Helper()
	resp := doPost(t, fmt.Sprintf("%s/api/endpoints/%d/probe", baseURL, endpointID), nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("probe endpoint %d: expected 200, got %d", endpointID, resp.StatusCode)
	}
}

// TestImageParamSeededRuleAppliesOnAllProbePaths is the core acceptance test
// of GH #33 (spec 0014 US 16-17): the seeded gpt-image rule appends
// quality:"low" to gpt-image probe requests while unmatched models keep the
// minimal body {model, prompt, n:1} — on all three call sites (discovery
// trial, manual model creation trial, and the scheduled/manual probe round)
// and on both image protocols.
func TestImageParamSeededRuleAppliesOnAllProbePaths(t *testing.T) {
	db := openTempDB(t)
	ts := newTestAPIServer(t, db)

	stub := newDiscoveryStubHub(t, []string{"gpt-image-2", "dall-unlisted"})
	stub.setImageMode("gpt-image-2", "success")
	stub.setEditMode("gpt-image-2", "success")
	stub.setImageMode("dall-unlisted", "success")
	stub.setEditMode("dall-unlisted", "success")

	// Path 1: discovery sync trial.
	hubID := createHubViaAPI(t, ts.URL, stub.URL)
	waitForHubSyncStatus(t, ts.URL, hubID, "succeeded")

	raw := lastGenerationRaw(t, stub, "gpt-image-2")
	if raw["quality"] != "low" {
		t.Errorf("discovery trial (gpt-image-2): expected quality=low, body %v", raw)
	}
	raw = lastGenerationRaw(t, stub, "dall-unlisted")
	if len(raw) != 3 || raw["model"] != "dall-unlisted" {
		t.Errorf("discovery trial (dall-unlisted): expected minimal body {model,prompt,n}, got %v", raw)
	}
	fields := lastEditFields(t, stub, "gpt-image-2")
	if fields["quality"] != "low" {
		t.Errorf("discovery edit trial (gpt-image-2): expected quality=low, fields %v", fields)
	}
	fields = lastEditFields(t, stub, "dall-unlisted")
	if len(fields) != 2 || fields["model"] != "dall-unlisted" || fields["prompt"] == "" {
		t.Errorf("discovery edit trial (dall-unlisted): expected only model+prompt fields, got %v", fields)
	}

	// Path 2: manual model creation trial (server trialProtocols).
	stub.setImageMode("gpt-image-manual", "success")
	resp := doPost(t, ts.URL+"/api/models", map[string]interface{}{
		"hub_id":   hubID,
		"model_id": "gpt-image-manual",
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create gpt-image-manual: expected 201, got %d", resp.StatusCode)
	}
	raw = lastGenerationRaw(t, stub, "gpt-image-manual")
	if raw["quality"] != "low" {
		t.Errorf("manual create trial (gpt-image-manual): expected quality=low, body %v", raw)
	}

	// Path 3: the probe round (prober) on both protocols.
	models := listModelsViaAPI(t, ts.URL)
	genID := int(endpointByProtocol(t, models["gpt-image-2"], "images_generation")["id"].(float64))
	editID := int(endpointByProtocol(t, models["gpt-image-2"], "images_edit")["id"].(float64))
	dallGenID := int(endpointByProtocol(t, models["dall-unlisted"], "images_generation")["id"].(float64))

	probeImageEndpoint(t, ts.URL, genID)
	raw = lastGenerationRaw(t, stub, "gpt-image-2")
	if raw["quality"] != "low" {
		t.Errorf("probe round (gpt-image-2 generations): expected quality=low, body %v", raw)
	}
	probeImageEndpoint(t, ts.URL, editID)
	fields = lastEditFields(t, stub, "gpt-image-2")
	if fields["quality"] != "low" {
		t.Errorf("probe round (gpt-image-2 edits): expected quality=low, fields %v", fields)
	}
	probeImageEndpoint(t, ts.URL, dallGenID)
	raw = lastGenerationRaw(t, stub, "dall-unlisted")
	if len(raw) != 3 || raw["model"] != "dall-unlisted" {
		t.Errorf("probe round (dall-unlisted): expected minimal body {model,prompt,n}, got %v", raw)
	}
}
