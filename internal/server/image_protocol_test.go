package server_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/taliove/hubscope/internal/hubclient"
	"github.com/taliove/hubscope/internal/scheduler"
)

// endpointByProtocol returns the model's endpoint object for the protocol.
func endpointByProtocol(t *testing.T, model map[string]interface{}, protocol string) map[string]interface{} {
	t.Helper()
	for _, raw := range model["endpoints"].([]interface{}) {
		ep := raw.(map[string]interface{})
		if ep["protocol"].(string) == protocol {
			return ep
		}
	}
	t.Fatalf("model %v has no %s endpoint", model["model_id"], protocol)
	return nil
}

// endpointInterval reads the interval_seconds override of an endpoint object.
func endpointInterval(t *testing.T, ep map[string]interface{}) interface{} {
	t.Helper()
	v, ok := ep["interval_seconds"]
	if !ok {
		t.Fatalf("endpoint %v misses interval_seconds", ep)
	}
	return v
}

// createImageEndpointViaDiscovery registers a hub whose listing contains the
// given image-capable model, waits for the first sync to finish, and returns
// the model's API object. The caller controls the image trial outcome via
// stub.setImageMode beforehand.
func createImageEndpointViaDiscovery(t *testing.T, baseURL string, stub *discoveryStubHub, modelID string) map[string]interface{} {
	t.Helper()
	stub.setModels([]string{modelID})
	hubID := createHubViaAPI(t, baseURL, stub.URL)
	waitForHubSyncStatus(t, baseURL, hubID, "succeeded")
	models := listModelsViaAPI(t, baseURL)
	model, ok := models[modelID]
	if !ok {
		t.Fatalf("model %s not discovered", modelID)
	}
	return model
}

// TestImageDiscoveryCreatesGenerationEndpoint covers the discovery path of
// spec 0014: an image-capable model whose image trial succeeds gains an
// enabled images_generation endpoint with the 1800s interval override, chat
// endpoints keep a null override, and non-image models are never sent an
// image trial.
func TestImageDiscoveryCreatesGenerationEndpoint(t *testing.T) {
	db := openTempDB(t)
	ts := newTestAPIServer(t, db)

	stub := newDiscoveryStubHub(t, []string{"gpt-image-2", "gpt-5"})
	stub.setFailing("gpt-5", "anthropic")
	stub.setImageMode("gpt-image-2", "success")
	hubID := createHubViaAPI(t, ts.URL, stub.URL)
	waitForHubSyncStatus(t, ts.URL, hubID, "succeeded")

	models := listModelsViaAPI(t, ts.URL)

	img := models["gpt-image-2"]
	if img["capability"] != "image" {
		t.Fatalf("gpt-image-2 capability: expected image, got %v", img["capability"])
	}
	imageEp := endpointByProtocol(t, img, "images_generation")
	if !imageEp["enabled"].(bool) {
		t.Error("images_generation endpoint should be enabled after a successful trial")
	}
	if got := endpointInterval(t, imageEp); got != float64(1800) {
		t.Errorf("images_generation interval_seconds: expected 1800, got %v", got)
	}
	for _, protocol := range []string{"anthropic", "openai"} {
		chatEp := endpointByProtocol(t, img, protocol)
		if got := endpointInterval(t, chatEp); got != nil {
			t.Errorf("%s interval_seconds: expected null (global default), got %v", protocol, got)
		}
	}

	// The trial request must carry the minimal contract: model + prompt + n=1.
	reqs := stub.imageRequests("gpt-image-2")
	if len(reqs) == 0 {
		t.Fatal("stub saw no images_generation trial for gpt-image-2")
	}
	last := reqs[len(reqs)-1]
	if last.Model != "gpt-image-2" || last.Prompt == "" || last.N != 1 {
		t.Errorf("image trial request shape: got %+v, want model=gpt-image-2, non-empty prompt, n=1", last)
	}

	// A non-image model is never trial-probed on the image protocol (user
	// story: no pointless trial cost or upstream noise).
	gpt5 := models["gpt-5"]
	if hasEndpoint(gpt5, "images_generation") {
		t.Error("gpt-5 (capability chat) must not get an images_generation endpoint")
	}
	if got := stub.imageRequests("gpt-5"); len(got) != 0 {
		t.Errorf("gpt-5 received %d image trial requests, want 0", len(got))
	}
}

// TestImageDiscoveryTrialFailureLeavesNoEndpoint pins ticket 17 semantics for
// the image protocol: a failed image trial creates no placeholder endpoint
// while the chat endpoints are unaffected.
func TestImageDiscoveryTrialFailureLeavesNoEndpoint(t *testing.T) {
	db := openTempDB(t)
	ts := newTestAPIServer(t, db)

	// No setImageMode: the image trial defaults to 503.
	stub := newDiscoveryStubHub(t, []string{"gpt-image-2"})
	hubID := createHubViaAPI(t, ts.URL, stub.URL)
	waitForHubSyncStatus(t, ts.URL, hubID, "succeeded")

	models := listModelsViaAPI(t, ts.URL)
	img := models["gpt-image-2"]
	if hasEndpoint(img, "images_generation") {
		t.Error("failed image trial must leave no images_generation endpoint")
	}
	if !hasEndpoint(img, "anthropic") || !hasEndpoint(img, "openai") {
		t.Error("chat endpoints must be unaffected by the failed image trial")
	}

	// Once the image path heals, the next sync backfills the endpoint (with
	// the interval override), same as any missing protocol.
	stub.setImageMode("gpt-image-2", "success")
	stats := runDiscovery(t, ts.URL)
	if got := statNumber(t, stats, "endpoints_created"); got != 1 {
		t.Errorf("backfill sync endpoints_created: expected 1, got %d", got)
	}
	models = listModelsViaAPI(t, ts.URL)
	imageEp := endpointByProtocol(t, models["gpt-image-2"], "images_generation")
	if got := endpointInterval(t, imageEp); got != float64(1800) {
		t.Errorf("backfilled images_generation interval_seconds: expected 1800, got %v", got)
	}
}

// TestImageManualCreateAndTrial covers user story 8 of spec 0014: a manually
// registered image-capable model is trial-probed on images_generation too,
// and the manual trial endpoint backfills it later with the interval
// override.
func TestImageManualCreateAndTrial(t *testing.T) {
	db := openTempDB(t)
	ts := newTestAPIServer(t, db)
	stub := newDiscoveryStubHub(t, nil)
	hubID := createHubViaAPI(t, ts.URL, stub.URL)
	waitForHubSyncStatus(t, ts.URL, hubID, "succeeded")

	// Manual creation with the image path answering: three endpoints.
	stub.setImageMode("dall-manual-ok", "success")
	resp := doPost(t, ts.URL+"/api/models", map[string]interface{}{
		"hub_id":   hubID,
		"model_id": "dall-manual-ok",
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create dall-manual-ok: expected 201, got %d", resp.StatusCode)
	}
	models := listModelsViaAPI(t, ts.URL)
	imageEp := endpointByProtocol(t, models["dall-manual-ok"], "images_generation")
	if got := endpointInterval(t, imageEp); got != float64(1800) {
		t.Errorf("manual images_generation interval_seconds: expected 1800, got %v", got)
	}
	chatEp := endpointByProtocol(t, models["dall-manual-ok"], "openai")
	if got := endpointInterval(t, chatEp); got != nil {
		t.Errorf("manual openai interval_seconds: expected null, got %v", got)
	}

	// Manual creation with the image path down: chat endpoints only, then the
	// trial endpoint backfills images_generation once it heals.
	resp = doPost(t, ts.URL+"/api/models", map[string]interface{}{
		"hub_id":   hubID,
		"model_id": "flux-manual-heal",
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create flux-manual-heal: expected 201, got %d", resp.StatusCode)
	}
	models = listModelsViaAPI(t, ts.URL)
	healed := models["flux-manual-heal"]
	if hasEndpoint(healed, "images_generation") {
		t.Fatal("flux-manual-heal should start without an images_generation endpoint")
	}

	stub.setImageMode("flux-manual-heal", "success")
	status, result := trialModelViaAPI(t, ts.URL, int64(healed["id"].(float64)))
	if status != http.StatusOK {
		t.Fatalf("trial: expected 200, got %d", status)
	}
	created := createdProtocols(t, result)
	if len(created) != 1 || created[0] != "images_generation" {
		t.Fatalf("trial created_protocols: expected [images_generation], got %v", created)
	}
	models = listModelsViaAPI(t, ts.URL)
	backfilled := endpointByProtocol(t, models["flux-manual-heal"], "images_generation")
	if got := endpointInterval(t, backfilled); got != float64(1800) {
		t.Errorf("trial-backfilled images_generation interval_seconds: expected 1800, got %v", got)
	}
}

// TestImageProbeRoundSingleRecord covers the probe-round shape of spec 0014:
// an image endpoint produces exactly one probe per round (streaming=false,
// ttft null) with usage mapped into the token fields, while a chat endpoint
// of the same model still produces its two records.
func TestImageProbeRoundSingleRecord(t *testing.T) {
	db := openTempDB(t)
	ts := newTestAPIServer(t, db)
	stub := newDiscoveryStubHub(t, nil)
	stub.setImageMode("gpt-image-2", "success")

	img := createImageEndpointViaDiscovery(t, ts.URL, stub, "gpt-image-2")
	imageID := int(endpointByProtocol(t, img, "images_generation")["id"].(float64))
	openaiID := int(endpointByProtocol(t, img, "openai")["id"].(float64))

	resp := doPost(t, fmt.Sprintf("%s/api/endpoints/%d/probe", ts.URL, imageID), nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("image probe round: expected 200, got %d", resp.StatusCode)
	}
	var env envelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode probe round: %v", err)
	}
	var result map[string]interface{}
	if err := json.Unmarshal(env.Data, &result); err != nil {
		t.Fatalf("unmarshal probe round: %v", err)
	}
	results := result["results"].([]interface{})
	if len(results) != 1 {
		t.Fatalf("image round: expected exactly 1 probe record, got %d", len(results))
	}
	probe := results[0].(map[string]interface{})
	if probe["streaming"].(bool) {
		t.Error("image probe must be non-streaming")
	}
	if !probe["ok"].(bool) {
		t.Errorf("image probe should succeed, got error %v", probe["error_summary"])
	}
	if probe["http_status"].(float64) != 200 {
		t.Errorf("expected http_status 200, got %v", probe["http_status"])
	}
	if probe["ttft_ms"] != nil {
		t.Errorf("image probe ttft_ms must be null (no streaming concept), got %v", probe["ttft_ms"])
	}
	// usage maps into the existing token fields (stub: 12 in / 3 out).
	if probe["input_tokens"].(float64) != 12 || probe["output_tokens"].(float64) != 3 {
		t.Errorf("usage mapping: expected 12/3 tokens, got %v/%v",
			probe["input_tokens"], probe["output_tokens"])
	}

	// Persisted history: exactly one record for the round.
	if records := probeRecords(t, ts.URL, imageID); len(records) != 1 {
		t.Fatalf("image endpoint history: expected 1 record, got %d", len(records))
	}

	// The chat endpoint of the same model still probes twice per round.
	resp2 := doPost(t, fmt.Sprintf("%s/api/endpoints/%d/probe", ts.URL, openaiID), nil)
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("chat probe round: expected 200, got %d", resp2.StatusCode)
	}
	if records := probeRecords(t, ts.URL, openaiID); len(records) != 2 {
		t.Fatalf("chat endpoint history: expected 2 records, got %d", len(records))
	}
}

// TestImageProbeSuccessDetermination covers the success boundary of spec
// 0014: 503, 200 with empty data, and 200 with a malformed data payload all
// fail; only 200 with a complete data payload succeeds.
func TestImageProbeSuccessDetermination(t *testing.T) {
	db := openTempDB(t)
	ts := newTestAPIServer(t, db)
	stub := newDiscoveryStubHub(t, nil)
	stub.setImageMode("gpt-image-2", "success")

	img := createImageEndpointViaDiscovery(t, ts.URL, stub, "gpt-image-2")
	imageID := int(endpointByProtocol(t, img, "images_generation")["id"].(float64))

	probeOnce := func() map[string]interface{} {
		t.Helper()
		resp := doPost(t, fmt.Sprintf("%s/api/endpoints/%d/probe", ts.URL, imageID), nil)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("probe round: expected 200, got %d", resp.StatusCode)
		}
		var env envelope
		if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
			t.Fatalf("decode probe round: %v", err)
		}
		var result map[string]interface{}
		if err := json.Unmarshal(env.Data, &result); err != nil {
			t.Fatalf("unmarshal probe round: %v", err)
		}
		results := result["results"].([]interface{})
		if len(results) != 1 {
			t.Fatalf("expected 1 probe record, got %d", len(results))
		}
		return results[0].(map[string]interface{})
	}

	stub.setImageMode("gpt-image-2", "") // 503
	if p := probeOnce(); p["ok"].(bool) || p["http_status"].(float64) != 503 {
		t.Errorf("503 mode: expected failure with status 503, got ok=%v status=%v", p["ok"], p["http_status"])
	}

	stub.setImageMode("gpt-image-2", "empty_data")
	if p := probeOnce(); p["ok"].(bool) || p["http_status"].(float64) != 200 {
		t.Errorf("empty data: expected failure with status 200, got ok=%v status=%v", p["ok"], p["http_status"])
	}

	stub.setImageMode("gpt-image-2", "bad_shape")
	if p := probeOnce(); p["ok"].(bool) || p["http_status"].(float64) != 200 {
		t.Errorf("bad shape: expected failure with status 200, got ok=%v status=%v", p["ok"], p["http_status"])
	}

	stub.setImageMode("gpt-image-2", "success")
	if p := probeOnce(); !p["ok"].(bool) {
		t.Errorf("success mode: expected ok, got error %v", p["error_summary"])
	}
}

// TestImageAlertingLifecycle reuses the alerts_test pattern for an image
// endpoint: three consecutive failed rounds (one probe each) fire exactly one
// down alert naming the images_generation protocol, the status machine shows
// failing then down, and a recovery fires one recovered notice.
func TestImageAlertingLifecycle(t *testing.T) {
	db := openTempDB(t)
	ts := newTestAPIServer(t, db)
	lark := newStubLarkServer(t)
	stub := newDiscoveryStubHub(t, nil)
	stub.setImageMode("gpt-image-2", "success")

	putResp := doPut(t, ts.URL+"/api/settings", map[string]interface{}{
		"lark_webhook_url": lark.URL,
		"alert_enabled":    true,
	})
	putResp.Body.Close()
	if putResp.StatusCode != http.StatusOK {
		t.Fatalf("put settings: expected 200, got %d", putResp.StatusCode)
	}

	img := createImageEndpointViaDiscovery(t, ts.URL, stub, "gpt-image-2")
	imageID := int64(endpointByProtocol(t, img, "images_generation")["id"].(float64))

	stub.setImageMode("gpt-image-2", "") // image path dies with 503

	// Round 1: one failed probe — below the threshold of 3, status failing.
	runProbeRound(t, ts, imageID)
	if got := len(lark.messages()); got != 0 {
		t.Fatalf("after 1 failure: expected no messages, got %d", got)
	}
	entry := findEntry(t, fetchOverview(t, ts.URL), imageID)
	if entry.Status != "failing" {
		t.Errorf("after 1 failed round: expected status failing, got %q", entry.Status)
	}

	// Round 2: still below the threshold.
	runProbeRound(t, ts, imageID)
	if got := len(lark.messages()); got != 0 {
		t.Fatalf("after 2 failures: expected no messages, got %d", got)
	}

	// Round 3: the third consecutive failure fires exactly one down alert.
	runProbeRound(t, ts, imageID)
	msgs := lark.messages()
	if len(msgs) != 1 {
		t.Fatalf("after crossing threshold: expected 1 message, got %d", len(msgs))
	}
	for _, want := range []string{"gpt-image-2", "images_generation", "HTTP 503"} {
		if !strings.Contains(msgs[0], want) {
			t.Errorf("down alert should contain %q, got: %s", want, msgs[0])
		}
	}
	entry = findEntry(t, fetchOverview(t, ts.URL), imageID)
	if entry.Status != "down" {
		t.Errorf("after 3 failed rounds: expected status down, got %q", entry.Status)
	}

	// Round 4: the outage continues, no repeat alert.
	runProbeRound(t, ts, imageID)
	if got := len(lark.messages()); got != 1 {
		t.Fatalf("ongoing outage: expected still 1 message, got %d", got)
	}

	events := listAlerts(t, ts, "")
	if len(events) != 1 || events[0]["kind"].(string) != "down" {
		t.Fatalf("expected exactly 1 down alert event, got %v", events)
	}

	// Recovery: the image path heals — one recovered notice.
	stub.setImageMode("gpt-image-2", "success")
	runProbeRound(t, ts, imageID)
	msgs = lark.messages()
	if len(msgs) != 2 {
		t.Fatalf("after recovery: expected 2 messages, got %d", len(msgs))
	}
	if !strings.Contains(msgs[1], "gpt-image-2") {
		t.Errorf("recovered alert should name the model, got: %s", msgs[1])
	}
	events = listAlerts(t, ts, "")
	if len(events) != 2 || events[0]["kind"].(string) != "recovered" {
		t.Fatalf("expected newest event kind recovered, got %v", events)
	}
}

// TestImageEndpointSchedulerInterval proves the 1800s interval override
// end-to-end: the scheduler fires the image endpoint's startup round (one
// probe), stays quiet across a 300s advance, and fires the second round only
// once 1800s have elapsed.
func TestImageEndpointSchedulerInterval(t *testing.T) {
	db := openTempDB(t)
	ts := newTestAPIServer(t, db)
	stub := newDiscoveryStubHub(t, nil)
	stub.setImageMode("gpt-image-2", "success")

	img := createImageEndpointViaDiscovery(t, ts.URL, stub, "gpt-image-2")
	imageID := int(endpointByProtocol(t, img, "images_generation")["id"].(float64))

	clock := scheduler.NewFakeClock(time.Now())
	startScheduler(t, db, hubclient.New(), clock)

	// Startup: the image endpoint is due immediately — exactly one record.
	waitForProbeCount(t, ts.URL, imageID, 1)

	// A chat-interval advance must not fire the image endpoint again.
	clock.Advance(300 * time.Second)
	assertProbeCountStable(t, ts.URL, imageID, 1, 300*time.Millisecond)

	// Reaching 1800s since the last completion triggers round 2.
	clock.Advance(1500 * time.Second)
	waitForProbeCount(t, ts.URL, imageID, 2)
}

// TestEvalNeverUsesImageProtocol is the R1 guard of spec 0014: evaluation
// traffic must stay on chat protocols. A chat-classified model whose only
// enabled endpoint is images_generation is recorded as "no enabled endpoint"
// instead of burning image-generation calls for text cases.
func TestEvalNeverUsesImageProtocol(t *testing.T) {
	db := openTempDB(t)
	ts := newTestAPIServer(t, db)
	stub := newDiscoveryStubHub(t, nil)
	hubID := createHubViaAPI(t, ts.URL, stub.URL)
	waitForHubSyncStatus(t, ts.URL, hubID, "succeeded")

	// Manual image model: chat + image endpoints after a successful trial.
	stub.setImageMode("dall-guard", "success")
	resp := doPost(t, ts.URL+"/api/models", map[string]interface{}{
		"hub_id":   hubID,
		"model_id": "dall-guard",
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create dall-guard: expected 201, got %d", resp.StatusCode)
	}
	imageReqsBefore := len(stub.imageRequests("dall-guard"))

	// Reclassify the model to chat (a higher-priority rule beats the seeded
	// dall→image rule) so the eval API accepts it, then disable both chat
	// endpoints: images_generation is the only enabled endpoint left.
	ruleResp := doPost(t, ts.URL+"/api/classification-rules", map[string]interface{}{
		"dimension": "capability",
		"keyword":   "dall-guard",
		"category":  "chat",
		"priority":  10,
	})
	ruleResp.Body.Close()
	if ruleResp.StatusCode != http.StatusCreated {
		t.Fatalf("create capability rule: expected 201, got %d", ruleResp.StatusCode)
	}

	models := listModelsViaAPI(t, ts.URL)
	model := models["dall-guard"]
	if model["capability"] != "chat" {
		t.Fatalf("dall-guard capability after reclassification: expected chat, got %v", model["capability"])
	}
	modelDBID := int64(model["id"].(float64))
	for _, protocol := range []string{"anthropic", "openai"} {
		epID := int(endpointByProtocol(t, model, protocol)["id"].(float64))
		patchResp := patchEndpoint(t, ts.URL, epID, map[string]interface{}{"enabled": false})
		patchResp.Body.Close()
		if patchResp.StatusCode != http.StatusOK {
			t.Fatalf("disable %s endpoint: expected 200, got %d", protocol, patchResp.StatusCode)
		}
	}

	suiteID := suiteIDByKey(t, ts.URL, "basic")
	runID := triggerEval(t, ts.URL, suiteID, modelDBID)
	run := waitEvalDone(t, ts.URL, runID)
	if run["status"] != "done" {
		t.Fatalf("eval run status: expected done, got %v", run["status"])
	}

	results := resultsByModel(run, "dall-guard")
	if len(results) == 0 {
		t.Fatal("expected failed results for every case of the suite")
	}
	for _, r := range results {
		if r["score"] != nil {
			t.Errorf("image-only model must not be scored, got %v", r["score"])
		}
		detail, _ := r["verdict_detail"].(string)
		if !strings.Contains(detail, "no enabled endpoint") {
			t.Errorf("verdict_detail should explain the missing chat endpoint, got %q", detail)
		}
	}

	// The guard is about traffic, not just outcomes: no image call may have
	// left the process during the whole eval run.
	if got := len(stub.imageRequests("dall-guard")); got != imageReqsBefore {
		t.Errorf("image requests during eval: expected %d (creation trial only), got %d", imageReqsBefore, got)
	}
}
