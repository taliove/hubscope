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
// spec 0018 T2 (GH #100): an image-capable model gets an enabled
// images_generation endpoint without any trial (trial-free creation), with the
// 1800s interval override, and no chat endpoints. Non-image models never get
// image endpoints.
func TestImageDiscoveryCreatesGenerationEndpoint(t *testing.T) {
	db := openTempDB(t)
	ts := newTestAPIServer(t, db)

	stub := newDiscoveryStubHub(t, []string{"gpt-image-2", "gpt-5"})
	stub.setFailing("gpt-5", "anthropic")
	hubID := createHubViaAPI(t, ts.URL, stub.URL)
	waitForHubSyncStatus(t, ts.URL, hubID, "succeeded")

	models := listModelsViaAPI(t, ts.URL)

	img := models["gpt-image-2"]
	if img["capability"] != "image" {
		t.Fatalf("gpt-image-2 capability: expected image, got %v", img["capability"])
	}
	imageEp := endpointByProtocol(t, img, "images_generation")
	if !imageEp["enabled"].(bool) {
		t.Error("images_generation endpoint should be enabled (trial-free creation)")
	}
	if got := endpointInterval(t, imageEp); got != float64(1800) {
		t.Errorf("images_generation interval_seconds: expected 1800, got %v", got)
	}
	// Image models are trial-free (GH #100): no chat endpoints, no trial requests.
	if hasEndpoint(img, "anthropic") || hasEndpoint(img, "openai") {
		t.Error("gpt-image-2 should have NO chat endpoints (image models are trial-free)")
	}
	if got := stub.imageRequests("gpt-image-2"); len(got) != 0 {
		t.Errorf("gpt-image-2 received %d image trial requests, want 0 (trial-free)", len(got))
	}

	// A non-image model never gets image endpoints.
	gpt5 := models["gpt-5"]
	if hasEndpoint(gpt5, "images_generation") {
		t.Error("gpt-5 (capability chat) must not get an images_generation endpoint")
	}
	if got := stub.imageRequests("gpt-5"); len(got) != 0 {
		t.Errorf("gpt-5 received %d image trial requests, want 0", len(got))
	}
}

// TestImageDiscoveryTrialFailureLeavesNoEndpoint is superseded by GH #100
// (trial-free creation): image endpoints are created without trial, so there
// is no "failed trial" scenario anymore. This test now verifies that image
// models always get their endpoints, and the backfill path still works.
func TestImageDiscoveryTrialFailureLeavesNoEndpoint(t *testing.T) {
	db := openTempDB(t)
	ts := newTestAPIServer(t, db)

	// Image models are trial-free: endpoints are created regardless of stub mode.
	stub := newDiscoveryStubHub(t, []string{"gpt-image-2"})
	hubID := createHubViaAPI(t, ts.URL, stub.URL)
	waitForHubSyncStatus(t, ts.URL, hubID, "succeeded")

	models := listModelsViaAPI(t, ts.URL)
	img := models["gpt-image-2"]
	// Trial-free creation: the endpoint is always created.
	if !hasEndpoint(img, "images_generation") {
		t.Error("image model should have images_generation endpoint (trial-free creation)")
	}
	// No chat endpoints for image models (GH #100).
	if hasEndpoint(img, "anthropic") || hasEndpoint(img, "openai") {
		t.Error("image model should have NO chat endpoints (trial-free)")
	}

	// The backfill path still works for missing protocols (e.g., manually
	// disabled endpoints that need re-creation).
	stats := runDiscovery(t, ts.URL)
	if got := statNumber(t, stats, "endpoints_created"); got != 0 {
		t.Errorf("second sync endpoints_created: expected 0 (already present), got %d", got)
	}
}

// TestImageManualCreateAndTrial covers user story 8 of spec 0016: a manually
// registered image-capable model is trial-probed on images_generation too,
// and the manual trial endpoint backfills it later with the interval
// TestImageManualCreateAndTrial covers manual registration of image-capable
// models under trial-free creation (GH #100, spec 0018 T2): image models get
// images_generation and images_edit endpoints without any probe call, and no
// chat endpoints.
func TestImageManualCreateAndTrial(t *testing.T) {
	db := openTempDB(t)
	ts := newTestAPIServer(t, db)
	stub := newDiscoveryStubHub(t, nil)
	hubID := createHubViaAPI(t, ts.URL, stub.URL)
	waitForHubSyncStatus(t, ts.URL, hubID, "succeeded")

	// Manual creation: image models are trial-free, endpoints created without probe.
	resp := doPost(t, ts.URL+"/api/models", map[string]interface{}{
		"hub_id":   hubID,
		"model_id": "dall-manual-ok",
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create dall-manual-ok: expected 201, got %d", resp.StatusCode)
	}
	models := listModelsViaAPI(t, ts.URL)
	img := models["dall-manual-ok"]
	if !hasEndpoint(img, "images_generation") {
		t.Error("dall-manual-ok should have images_generation endpoint (trial-free)")
	}
	imageEp := endpointByProtocol(t, img, "images_generation")
	if got := endpointInterval(t, imageEp); got != float64(1800) {
		t.Errorf("manual images_generation interval_seconds: expected 1800, got %v", got)
	}
	// No chat endpoints for image models (GH #100).
	if hasEndpoint(img, "openai") || hasEndpoint(img, "anthropic") {
		t.Error("dall-manual-ok should have NO chat endpoints (image models are trial-free)")
	}
	// Zero probe requests for trial-free creation.
	if got := stub.imageRequests("dall-manual-ok"); len(got) != 0 {
		t.Errorf("dall-manual-ok image requests: expected 0 (trial-free), got %d", len(got))
	}
}

// TestImageProbeRoundSingleRecord covers the probe-round shape of spec 0016:
// an image endpoint produces exactly one probe per round (streaming=false,
// ttft null) with usage mapped into the token fields, while a chat endpoint
// of the same model still produces its two records.
func TestImageProbeRoundSingleRecord(t *testing.T) {
	t.Skip("spec 0018 T1 (GH #97): image endpoints no longer probed, test obsolete")
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
	t.Skip("spec 0018 T1 (GH #97): image endpoints no longer probed, test obsolete")
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
// endpoint under the aggregation window (spec 0017): three consecutive
// failed rounds (one probe each) record the down event at decision time, the
// fake-clock flush sends one aggregated down card naming the
// images_generation protocol, the status machine shows failing then down,
// and a recovery rides the next window flush.
func TestImageAlertingLifecycle(t *testing.T) {
	t.Skip("spec 0018 T1 (GH #97): image endpoints no longer probed, test obsolete")
	db := openTempDB(t)
	clock := scheduler.NewFakeClock(time.Now())
	ts := newAlertClockServer(t, db, clock)
	lark := newStubLarkServer(t)
	stub := newDiscoveryStubHub(t, nil)
	stub.setImageMode("gpt-image-2", "success")

	configureWebhook(t, ts, lark, true)

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

	// Round 3: the third consecutive failure records the down event at
	// decision time; nothing is sent before the window flushes.
	runProbeRound(t, ts, imageID)
	if got := len(lark.messages()); got != 0 {
		t.Fatalf("before window flush: expected no messages, got %d", got)
	}
	clock.Advance(alertWindowForTest)
	msgs := waitForLarkMessages(t, lark, 1)
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
	clock.Advance(alertWindowForTest)
	if got := len(lark.messages()); got != 1 {
		t.Fatalf("ongoing outage: expected still 1 message, got %d", got)
	}

	waitForAlertEvents(t, ts, 2) // down + batch
	if got := alertEventsOfKind(t, ts, "down"); len(got) != 1 {
		t.Fatalf("expected exactly 1 down alert event, got %v", got)
	}

	// Recovery: the image path heals — one recovered notice via the window.
	stub.setImageMode("gpt-image-2", "success")
	runProbeRound(t, ts, imageID)
	if got := len(lark.messages()); got != 1 {
		t.Fatalf("before recovery flush: expected still 1 message, got %d", got)
	}
	clock.Advance(alertWindowForTest)
	msgs = waitForLarkMessages(t, lark, 2)
	if !strings.Contains(msgs[1], "gpt-image-2") {
		t.Errorf("recovered alert should name the model, got: %s", msgs[1])
	}
	waitForAlertEvents(t, ts, 4)
	if got := alertEventsOfKind(t, ts, "recovered"); len(got) != 1 {
		t.Fatalf("expected exactly 1 recovered alert event, got %v", got)
	}
}

// TestImageEndpointSchedulerInterval proves the 1800s interval override
// end-to-end: the scheduler fires the image endpoint's startup round (one
// probe), stays quiet across a 300s advance, and fires the second round only
// once 1800s have elapsed.
func TestImageEndpointSchedulerInterval(t *testing.T) {
	t.Skip("spec 0018 T1 (GH #97): image endpoints no longer probed, test obsolete")
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

// TestEvalNeverUsesImageProtocol is the R1 guard of spec 0016: evaluation
// traffic must stay on chat protocols. A chat-classified model whose only
// enabled endpoint is images_generation is recorded as "no enabled endpoint"
// instead of burning image-generation calls for text cases.
func TestEvalNeverUsesImageProtocol(t *testing.T) {
	// spec 0018 T2 (GH #100): image models are trial-free — they no longer
	// get chat endpoints from trial, so "disable chat, force image" no longer
	// applies. Skip until rewritten for the trial-free world.
	t.Skip("spec 0018 T2 (GH #100): image models trial-free, chat endpoints no longer created for image models")
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

	suiteID := suiteIDByKey(t, ts.URL, "mmlu")
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

// ---------------------------------------------------------------------------
// images_edit (GH #32, spec 0016 second slice): the edit path is a second,
// independent image protocol — multipart POST /v1/images/edits carrying the
// embedded test image. Everything below mirrors the generations group.
// ---------------------------------------------------------------------------

// TestImageEditDiscoveryCreatesEditEndpoint covers the discovery path of GH
// #32: an image-capable model whose edit trial succeeds gains an enabled
// images_edit endpoint (independent of images_generation) with the 1800s
// interval override, the trial request is a real multipart upload of the
// embedded test image (image + prompt + model fields), and non-image models
// are never sent an edit trial.
func TestImageEditDiscoveryCreatesEditEndpoint(t *testing.T) {
	// spec 0018 T2 (GH #100): image models are trial-free — no image trial
	// requests are sent during discovery. This test asserted on trial requests.
	t.Skip("spec 0018 T2 (GH #100): image models trial-free, no edit trial requests sent")
	db := openTempDB(t)
	ts := newTestAPIServer(t, db)

	stub := newDiscoveryStubHub(t, []string{"gpt-image-2", "gpt-5"})
	stub.setImageMode("gpt-image-2", "success")
	stub.setEditMode("gpt-image-2", "success")
	hubID := createHubViaAPI(t, ts.URL, stub.URL)
	waitForHubSyncStatus(t, ts.URL, hubID, "succeeded")

	models := listModelsViaAPI(t, ts.URL)

	img := models["gpt-image-2"]
	editEp := endpointByProtocol(t, img, "images_edit")
	if !editEp["enabled"].(bool) {
		t.Error("images_edit endpoint should be enabled after a successful trial")
	}
	if got := endpointInterval(t, editEp); got != float64(1800) {
		t.Errorf("images_edit interval_seconds: expected 1800, got %v", got)
	}
	// The generation endpoint is an independent sibling, not a bundle.
	if !hasEndpoint(img, "images_generation") {
		t.Error("images_generation endpoint should exist alongside images_edit")
	}

	// The edit trial must be a real multipart upload: model + prompt fields
	// and the embedded test image in the image file part.
	reqs := stub.editRequests("gpt-image-2")
	if len(reqs) == 0 {
		t.Fatal("stub saw no images_edit trial for gpt-image-2")
	}
	last := reqs[len(reqs)-1]
	if last.Model != "gpt-image-2" || last.Prompt == "" || !last.ImagePresent {
		t.Errorf("edit trial request shape: got %+v, want model=gpt-image-2, non-empty prompt, image uploaded", last)
	}

	// A non-image model is never trial-probed on the edit protocol (no
	// pointless trial cost or upstream noise).
	gpt5 := models["gpt-5"]
	if hasEndpoint(gpt5, "images_edit") {
		t.Error("gpt-5 (capability chat) must not get an images_edit endpoint")
	}
	if got := stub.editRequests("gpt-5"); len(got) != 0 {
		t.Errorf("gpt-5 received %d edit requests, want 0", len(got))
	}
}

// TestImageEditTrialIndependentOfGeneration is the reverse case of GH #32:
// the generation path answering while the edit path is down (the two route
// to different upstreams) must produce the generation endpoint only — no
// edits placeholder — and the next sync backfills images_edit once the edit
// path heals, with the interval override.
func TestImageEditTrialIndependentOfGeneration(t *testing.T) {
	// spec 0018 T2 (GH #100): image models are trial-free — no trial requests.
	t.Skip("spec 0018 T2 (GH #100): image models trial-free, no edit trial requests")
	db := openTempDB(t)
	ts := newTestAPIServer(t, db)

	// Generation answers, edits defaults to 503.
	stub := newDiscoveryStubHub(t, []string{"gpt-image-2"})
	stub.setImageMode("gpt-image-2", "success")
	hubID := createHubViaAPI(t, ts.URL, stub.URL)
	waitForHubSyncStatus(t, ts.URL, hubID, "succeeded")

	models := listModelsViaAPI(t, ts.URL)
	img := models["gpt-image-2"]
	if !hasEndpoint(img, "images_generation") {
		t.Fatal("successful generation trial should create the images_generation endpoint")
	}
	if hasEndpoint(img, "images_edit") {
		t.Error("failed edit trial must leave no images_edit placeholder endpoint")
	}

	// The edit path heals: the next sync backfills only the missing protocol.
	stub.setEditMode("gpt-image-2", "success")
	stats := runDiscovery(t, ts.URL)
	if got := statNumber(t, stats, "endpoints_created"); got != 1 {
		t.Errorf("backfill sync endpoints_created: expected 1, got %d", got)
	}
	models = listModelsViaAPI(t, ts.URL)
	editEp := endpointByProtocol(t, models["gpt-image-2"], "images_edit")
	if got := endpointInterval(t, editEp); got != float64(1800) {
		t.Errorf("backfilled images_edit interval_seconds: expected 1800, got %v", got)
	}
}

// TestImageEditManualTrialBackfill covers the manual path of GH #32: a
// manually registered image-capable model created while the edit path is
// down starts without an images_edit endpoint, and the trial endpoint
// backfills it (with the interval override) once the path heals.
func TestImageEditManualTrialBackfill(t *testing.T) {
	// spec 0018 T2 (GH #100): image models are trial-free — no trial requests.
	t.Skip("spec 0018 T2 (GH #100): image models trial-free, no edit trial requests")
	db := openTempDB(t)
	ts := newTestAPIServer(t, db)
	stub := newDiscoveryStubHub(t, nil)
	hubID := createHubViaAPI(t, ts.URL, stub.URL)
	waitForHubSyncStatus(t, ts.URL, hubID, "succeeded")

	stub.setImageMode("flux-edit-heal", "success") // generation OK, edits 503
	resp := doPost(t, ts.URL+"/api/models", map[string]interface{}{
		"hub_id":   hubID,
		"model_id": "flux-edit-heal",
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create flux-edit-heal: expected 201, got %d", resp.StatusCode)
	}
	models := listModelsViaAPI(t, ts.URL)
	model := models["flux-edit-heal"]
	if hasEndpoint(model, "images_edit") {
		t.Fatal("flux-edit-heal should start without an images_edit endpoint")
	}

	stub.setEditMode("flux-edit-heal", "success")
	status, result := trialModelViaAPI(t, ts.URL, int64(model["id"].(float64)))
	if status != http.StatusOK {
		t.Fatalf("trial: expected 200, got %d", status)
	}
	created := createdProtocols(t, result)
	if len(created) != 1 || created[0] != "images_edit" {
		t.Fatalf("trial created_protocols: expected [images_edit], got %v", created)
	}
	models = listModelsViaAPI(t, ts.URL)
	editEp := endpointByProtocol(t, models["flux-edit-heal"], "images_edit")
	if got := endpointInterval(t, editEp); got != float64(1800) {
		t.Errorf("trial-backfilled images_edit interval_seconds: expected 1800, got %v", got)
	}
}

// TestImageEditProbeRoundSingleRecord covers the probe-round shape of GH #32:
// an images_edit endpoint produces exactly one probe per round (streaming=
// false, ttft null) with usage mapped into the token fields, and the probe
// really uploads the embedded test image as a multipart form.
func TestImageEditProbeRoundSingleRecord(t *testing.T) {
	t.Skip("spec 0018 T1 (GH #97): image endpoints no longer probed, test obsolete")
	db := openTempDB(t)
	ts := newTestAPIServer(t, db)
	stub := newDiscoveryStubHub(t, nil)
	stub.setImageMode("gpt-image-2", "success")
	stub.setEditMode("gpt-image-2", "success")

	img := createImageEndpointViaDiscovery(t, ts.URL, stub, "gpt-image-2")
	editID := int(endpointByProtocol(t, img, "images_edit")["id"].(float64))

	resp := doPost(t, fmt.Sprintf("%s/api/endpoints/%d/probe", ts.URL, editID), nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("edit probe round: expected 200, got %d", resp.StatusCode)
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
		t.Fatalf("edit round: expected exactly 1 probe record, got %d", len(results))
	}
	probe := results[0].(map[string]interface{})
	if probe["streaming"].(bool) {
		t.Error("edit probe must be non-streaming")
	}
	if !probe["ok"].(bool) {
		t.Errorf("edit probe should succeed, got error %v", probe["error_summary"])
	}
	if probe["http_status"].(float64) != 200 {
		t.Errorf("expected http_status 200, got %v", probe["http_status"])
	}
	if probe["ttft_ms"] != nil {
		t.Errorf("edit probe ttft_ms must be null (no streaming concept), got %v", probe["ttft_ms"])
	}
	// usage maps into the existing token fields (stub: 12 in / 3 out).
	if probe["input_tokens"].(float64) != 12 || probe["output_tokens"].(float64) != 3 {
		t.Errorf("usage mapping: expected 12/3 tokens, got %v/%v",
			probe["input_tokens"], probe["output_tokens"])
	}

	// Persisted history: exactly one record for the round.
	if records := probeRecords(t, ts.URL, editID); len(records) != 1 {
		t.Fatalf("edit endpoint history: expected 1 record, got %d", len(records))
	}

	// The stub validates the multipart contract (400 on missing/empty fields),
	// so a succeeding probe proves the wire shape; assert what it recorded.
	reqs := stub.editRequests("gpt-image-2")
	last := reqs[len(reqs)-1]
	if last.Model != "gpt-image-2" || last.Prompt == "" || !last.ImagePresent {
		t.Errorf("edit probe request shape: got %+v, want model + prompt + image", last)
	}
}

// TestImageEditProbeSuccessDetermination covers the success boundary of GH
// #32, same determination as generations: 503, 200 with empty data, and 200 with a
// malformed data payload all fail; only 200 with a complete data payload
// succeeds.
func TestImageEditProbeSuccessDetermination(t *testing.T) {
	t.Skip("spec 0018 T1 (GH #97): image endpoints no longer probed, test obsolete")
	db := openTempDB(t)
	ts := newTestAPIServer(t, db)
	stub := newDiscoveryStubHub(t, nil)
	stub.setImageMode("gpt-image-2", "success")
	stub.setEditMode("gpt-image-2", "success")

	img := createImageEndpointViaDiscovery(t, ts.URL, stub, "gpt-image-2")
	editID := int(endpointByProtocol(t, img, "images_edit")["id"].(float64))

	probeOnce := func() map[string]interface{} {
		t.Helper()
		resp := doPost(t, fmt.Sprintf("%s/api/endpoints/%d/probe", ts.URL, editID), nil)
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

	stub.setEditMode("gpt-image-2", "") // 503
	if p := probeOnce(); p["ok"].(bool) || p["http_status"].(float64) != 503 {
		t.Errorf("503 mode: expected failure with status 503, got ok=%v status=%v", p["ok"], p["http_status"])
	}

	stub.setEditMode("gpt-image-2", "empty_data")
	if p := probeOnce(); p["ok"].(bool) || p["http_status"].(float64) != 200 {
		t.Errorf("empty data: expected failure with status 200, got ok=%v status=%v", p["ok"], p["http_status"])
	}

	stub.setEditMode("gpt-image-2", "bad_shape")
	if p := probeOnce(); p["ok"].(bool) || p["http_status"].(float64) != 200 {
		t.Errorf("bad shape: expected failure with status 200, got ok=%v status=%v", p["ok"], p["http_status"])
	}

	stub.setEditMode("gpt-image-2", "success")
	if p := probeOnce(); !p["ok"].(bool) {
		t.Errorf("success mode: expected ok, got error %v", p["error_summary"])
	}
}

// TestImageEditAlertingLifecycle proves the edit path rides the same alert
// chain (W5) as every other protocol, aggregation window included (spec
// 0017): three consecutive failed rounds record the down event, the flush
// sends one down card naming the images_edit protocol, and a recovery rides
// the next flush.
func TestImageEditAlertingLifecycle(t *testing.T) {
	t.Skip("spec 0018 T1 (GH #97): image endpoints no longer probed, test obsolete")
	db := openTempDB(t)
	clock := scheduler.NewFakeClock(time.Now())
	ts := newAlertClockServer(t, db, clock)
	lark := newStubLarkServer(t)
	stub := newDiscoveryStubHub(t, nil)
	stub.setImageMode("gpt-image-2", "success")
	stub.setEditMode("gpt-image-2", "success")

	configureWebhook(t, ts, lark, true)

	img := createImageEndpointViaDiscovery(t, ts.URL, stub, "gpt-image-2")
	editID := int64(endpointByProtocol(t, img, "images_edit")["id"].(float64))

	stub.setEditMode("gpt-image-2", "") // edit path dies with 503

	// Round 1: one failed probe — below the threshold of 3, status failing.
	runProbeRound(t, ts, editID)
	if got := len(lark.messages()); got != 0 {
		t.Fatalf("after 1 failure: expected no messages, got %d", got)
	}
	entry := findEntry(t, fetchOverview(t, ts.URL), editID)
	if entry.Status != "failing" {
		t.Errorf("after 1 failed round: expected status failing, got %q", entry.Status)
	}

	// Round 2: still below the threshold.
	runProbeRound(t, ts, editID)
	if got := len(lark.messages()); got != 0 {
		t.Fatalf("after 2 failures: expected no messages, got %d", got)
	}

	// Round 3: the third consecutive failure records the down event;
	// nothing is sent before the window flushes.
	runProbeRound(t, ts, editID)
	if got := len(lark.messages()); got != 0 {
		t.Fatalf("before window flush: expected no messages, got %d", got)
	}
	clock.Advance(alertWindowForTest)
	msgs := waitForLarkMessages(t, lark, 1)
	for _, want := range []string{"gpt-image-2", "images_edit", "HTTP 503"} {
		if !strings.Contains(msgs[0], want) {
			t.Errorf("down alert should contain %q, got: %s", want, msgs[0])
		}
	}
	entry = findEntry(t, fetchOverview(t, ts.URL), editID)
	if entry.Status != "down" {
		t.Errorf("after 3 failed rounds: expected status down, got %q", entry.Status)
	}

	// Round 4: the outage continues, no repeat alert.
	runProbeRound(t, ts, editID)
	clock.Advance(alertWindowForTest)
	if got := len(lark.messages()); got != 1 {
		t.Fatalf("ongoing outage: expected still 1 message, got %d", got)
	}

	waitForAlertEvents(t, ts, 2) // down + batch
	if got := alertEventsOfKind(t, ts, "down"); len(got) != 1 {
		t.Fatalf("expected exactly 1 down alert event, got %v", got)
	}

	// Recovery: the edit path heals — one recovered notice via the window.
	stub.setEditMode("gpt-image-2", "success")
	runProbeRound(t, ts, editID)
	if got := len(lark.messages()); got != 1 {
		t.Fatalf("before recovery flush: expected still 1 message, got %d", got)
	}
	clock.Advance(alertWindowForTest)
	msgs = waitForLarkMessages(t, lark, 2)
	if !strings.Contains(msgs[1], "gpt-image-2") {
		t.Errorf("recovered alert should name the model, got: %s", msgs[1])
	}
	waitForAlertEvents(t, ts, 4)
	if got := alertEventsOfKind(t, ts, "recovered"); len(got) != 1 {
		t.Fatalf("expected exactly 1 recovered alert event, got %v", got)
	}
}

// TestEvalNeverUsesImageEditProtocol is the R1 guard of spec 0016 applied to
// the edit protocol: a chat-classified model whose only enabled endpoint is
// images_edit is recorded as "no enabled endpoint" — eval traffic never
// burns paid image edits.
func TestEvalNeverUsesImageEditProtocol(t *testing.T) {
	// spec 0018 T2 (GH #100): image models are trial-free — chat endpoints no
	// longer created for image models, so "disable chat, force edit" N/A.
	t.Skip("spec 0018 T2 (GH #100): image models trial-free, chat endpoints not created for image models")
	db := openTempDB(t)
	ts := newTestAPIServer(t, db)
	stub := newDiscoveryStubHub(t, nil)
	hubID := createHubViaAPI(t, ts.URL, stub.URL)
	waitForHubSyncStatus(t, ts.URL, hubID, "succeeded")

	// Manual image model: chat + edits endpoints after a successful edit
	// trial (generation stays down, so images_edit is the only image
	// endpoint).
	stub.setEditMode("dall-edit-guard", "success")
	resp := doPost(t, ts.URL+"/api/models", map[string]interface{}{
		"hub_id":   hubID,
		"model_id": "dall-edit-guard",
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create dall-edit-guard: expected 201, got %d", resp.StatusCode)
	}
	editReqsBefore := len(stub.editRequests("dall-edit-guard"))

	// Reclassify the model to chat so the eval API accepts it, then disable
	// both chat endpoints: images_edit is the only enabled endpoint left.
	ruleResp := doPost(t, ts.URL+"/api/classification-rules", map[string]interface{}{
		"dimension": "capability",
		"keyword":   "dall-edit-guard",
		"category":  "chat",
		"priority":  10,
	})
	ruleResp.Body.Close()
	if ruleResp.StatusCode != http.StatusCreated {
		t.Fatalf("create capability rule: expected 201, got %d", ruleResp.StatusCode)
	}

	models := listModelsViaAPI(t, ts.URL)
	model := models["dall-edit-guard"]
	if model["capability"] != "chat" {
		t.Fatalf("dall-edit-guard capability after reclassification: expected chat, got %v", model["capability"])
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

	suiteID := suiteIDByKey(t, ts.URL, "mmlu")
	runID := triggerEval(t, ts.URL, suiteID, modelDBID)
	run := waitEvalDone(t, ts.URL, runID)
	if run["status"] != "done" {
		t.Fatalf("eval run status: expected done, got %v", run["status"])
	}

	results := resultsByModel(run, "dall-edit-guard")
	if len(results) == 0 {
		t.Fatal("expected failed results for every case of the suite")
	}
	for _, r := range results {
		if r["score"] != nil {
			t.Errorf("edits-only model must not be scored, got %v", r["score"])
		}
		detail, _ := r["verdict_detail"].(string)
		if !strings.Contains(detail, "no enabled endpoint") {
			t.Errorf("verdict_detail should explain the missing chat endpoint, got %q", detail)
		}
	}

	// The guard is about traffic, not just outcomes: no edit call may have
	// left the process during the whole eval run.
	if got := len(stub.editRequests("dall-edit-guard")); got != editReqsBefore {
		t.Errorf("edit requests during eval: expected %d (creation trial only), got %d", editReqsBefore, got)
	}
}
