package server_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// discoveryStubHub is a stub Hub whose /v1/models listing can change between
// syncs and whose probe success can be controlled per (model, protocol).
type discoveryStubHub struct {
	*httptest.Server

	mu           sync.Mutex
	modelIDs     []string
	failing      map[string]map[string]bool // modelID -> protocol -> should fail
	lastListAuth string                     // Authorization header of the last /v1/models call
	listFails    bool                       // /v1/models answers HTTP 500
	hold         chan struct{}              // non-nil: /v1/models blocks until released
	// imageModes controls the /v1/images/generations answer per model:
	// "success" / "empty_data" / "bad_shape"; an absent entry answers 503
	// (spec 0014: the image trial must default to failure so existing
	// assertions on chat-only endpoint sets keep holding).
	imageModes map[string]string
	// imageReqs records every /v1/images/generations request body per model,
	// so tests can assert the wire shape and that no image traffic happened.
	imageReqs map[string][]imageRequest
	// editModes controls the /v1/images/edits answer per model, with the same
	// values and 503 default as imageModes. The map is independent of
	// imageModes so tests can express "generation works, edits does not"
	// (and the reverse) — the two upstream paths fail separately in reality.
	editModes map[string]string
	// editReqs records every /v1/images/edits call per model: model, prompt,
	// and whether a non-empty image file arrived. Headers (notably
	// Authorization) are deliberately not recorded (W6).
	editReqs map[string][]editRequest
}

// imageRequest is the recorded body of one /v1/images/generations call.
type imageRequest struct {
	Model  string
	Prompt string
	N      int
}

// editRequest is the recorded shape of one /v1/images/edits call: the
// multipart fields the contract cares about, nothing else.
type editRequest struct {
	Model        string
	Prompt       string
	ImagePresent bool
}

func newDiscoveryStubHub(t *testing.T, modelIDs []string) *discoveryStubHub {
	t.Helper()
	stub := &discoveryStubHub{
		modelIDs:   modelIDs,
		failing:    map[string]map[string]bool{},
		imageModes: map[string]string{},
		imageReqs:  map[string][]imageRequest{},
		editModes:  map[string]string{},
		editReqs:   map[string][]editRequest{},
	}
	stub.Server = httptest.NewServer(http.HandlerFunc(stub.handle))
	t.Cleanup(stub.Close)
	return stub
}

// setImageMode makes /v1/images/generations for the model answer per mode
// ("success" / "empty_data" / "bad_shape"); any other value (including the
// zero value) answers 503.
func (s *discoveryStubHub) setImageMode(modelID, mode string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.imageModes[modelID] = mode
}

// imageRequests returns the recorded /v1/images/generations request bodies
// for the model, in arrival order.
func (s *discoveryStubHub) imageRequests(modelID string) []imageRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]imageRequest(nil), s.imageReqs[modelID]...)
}

// setEditMode makes /v1/images/edits for the model answer per mode (same
// values as setImageMode); any other value (including the zero value)
// answers 503.
func (s *discoveryStubHub) setEditMode(modelID, mode string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.editModes[modelID] = mode
}

// editRequests returns the recorded /v1/images/edits call shapes for the
// model, in arrival order.
func (s *discoveryStubHub) editRequests(modelID string) []editRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]editRequest(nil), s.editReqs[modelID]...)
}

// setModels replaces the model list returned by /v1/models.
func (s *discoveryStubHub) setModels(ids []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.modelIDs = ids
}

// setFailing makes probes for the given model+protocol return HTTP 503.
func (s *discoveryStubHub) setFailing(modelID, protocol string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failing[modelID] == nil {
		s.failing[modelID] = map[string]bool{}
	}
	s.failing[modelID][protocol] = true
}

// clearFailing makes probes for the given model+protocol succeed again.
func (s *discoveryStubHub) clearFailing(modelID, protocol string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.failing[modelID], protocol)
}

// listAuth returns the Authorization header seen on the last /v1/models call.
func (s *discoveryStubHub) listAuth() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastListAuth
}

// setListFailing makes /v1/models answer HTTP 500 (or heal again).
func (s *discoveryStubHub) setListFailing(fail bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.listFails = fail
}

// holdList makes the next /v1/models call block until releaseList.
func (s *discoveryStubHub) holdList() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hold = make(chan struct{})
}

// releaseList unblocks a /v1/models call parked by holdList.
func (s *discoveryStubHub) releaseList() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.hold != nil {
		close(s.hold)
		s.hold = nil
	}
}

func (s *discoveryStubHub) handle(w http.ResponseWriter, r *http.Request) {
	if strings.HasSuffix(r.URL.Path, "/v1/models") {
		s.mu.Lock()
		ids := append([]string(nil), s.modelIDs...)
		s.lastListAuth = r.Header.Get("Authorization")
		hold := s.hold
		fail := s.listFails
		s.mu.Unlock()

		if hold != nil {
			<-hold
		}
		if fail {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, `{"error":{"message":"listing unavailable"}}`)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		var sb strings.Builder
		sb.WriteString(`{"data":[`)
		for i, id := range ids {
			if i > 0 {
				sb.WriteString(",")
			}
			fmt.Fprintf(&sb, `{"id":%q}`, id)
		}
		sb.WriteString(`]}`)
		fmt.Fprint(w, sb.String())
		return
	}

	if strings.HasSuffix(r.URL.Path, "/v1/images/generations") {
		s.handleImageGeneration(w, r)
		return
	}

	// The edits route is multipart and must be intercepted before the chat
	// JSON parsing below — a multipart body fed to json.Unmarshal would be
	// silently misread as an empty model and pollute the chat failure maps.
	if strings.HasSuffix(r.URL.Path, "/v1/images/edits") {
		s.handleImageEdit(w, r)
		return
	}

	isAnthropic := strings.HasSuffix(r.URL.Path, "/v1/messages")
	protocol := "openai"
	if isAnthropic {
		protocol = "anthropic"
	}

	body, _ := io.ReadAll(r.Body)
	var req struct {
		Model  string `json:"model"`
		Stream bool   `json:"stream"`
	}
	_ = json.Unmarshal(body, &req)

	s.mu.Lock()
	fail := s.failing[req.Model][protocol]
	s.mu.Unlock()
	if fail {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprint(w, `{"error":{"message":"No available providers for this model"}}`)
		return
	}

	if req.Stream {
		writeStubStream(w, isAnthropic)
		return
	}
	writeStubNonStream(w, isAnthropic)
}

// handleImageGeneration answers POST /v1/images/generations. The request is
// always recorded, then validated against the minimal probe contract
// {model, prompt, n:1} (spec 0014): a malformed body answers 400 so a
// hardcoded-field implementation bug fails the trial instead of passing it.
// The response shape is controlled per model via setImageMode and defaults
// to 503.
func (s *discoveryStubHub) handleImageGeneration(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	var req struct {
		Model  string `json:"model"`
		Prompt string `json:"prompt"`
		N      int    `json:"n"`
	}
	_ = json.Unmarshal(body, &req)

	s.mu.Lock()
	s.imageReqs[req.Model] = append(s.imageReqs[req.Model], imageRequest{
		Model: req.Model, Prompt: req.Prompt, N: req.N,
	})
	mode := s.imageModes[req.Model]
	fail := s.failing[req.Model]["images_generation"]
	s.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	if req.Model == "" || req.Prompt == "" || req.N != 1 {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"error":{"message":"invalid image generation request"}}`)
		return
	}
	if fail {
		mode = ""
	}
	switch mode {
	case "success":
		fmt.Fprint(w, `{"created":1720000000,"data":[{"b64_json":"aGVsbG8="}],`+
			`"usage":{"total_tokens":15,"input_tokens":12,"output_tokens":3}}`)
	case "empty_data":
		fmt.Fprint(w, `{"created":1720000000,"data":[]}`)
	case "bad_shape":
		fmt.Fprint(w, `{"created":1720000000,"data":[{"revised_prompt":"painted square"}]}`)
	default:
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprint(w, `{"error":{"message":"No available providers for this model"}}`)
	}
}

// handleImageEdit answers POST /v1/images/edits. The multipart form is really
// parsed (ParseMultipartForm) so the test proves the probe actually uploads
// the embedded test image: model + prompt fields and a non-empty image file
// part are required, anything missing or empty answers 400 — a client with
// wrong field names (e.g. an image[]/file dialect) fails the trial instead
// of passing it. The call is always recorded (model/prompt/image presence
// only, never headers). The response shape is controlled per model via
// setEditMode and defaults to 503.
func (s *discoveryStubHub) handleImageEdit(w http.ResponseWriter, r *http.Request) {
	model, prompt, imagePresent := "", "", false
	if err := r.ParseMultipartForm(10 << 20); err == nil {
		model = r.FormValue("model")
		prompt = r.FormValue("prompt")
		if file, _, err := r.FormFile("image"); err == nil {
			n, _ := io.Copy(io.Discard, file)
			file.Close()
			imagePresent = n > 0
		}
	}

	s.mu.Lock()
	s.editReqs[model] = append(s.editReqs[model], editRequest{
		Model: model, Prompt: prompt, ImagePresent: imagePresent,
	})
	mode := s.editModes[model]
	fail := s.failing[model]["images_edit"]
	s.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	if model == "" || prompt == "" || !imagePresent {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"error":{"message":"invalid image edit request"}}`)
		return
	}
	if fail {
		mode = ""
	}
	switch mode {
	case "success":
		fmt.Fprint(w, `{"created":1720000000,"data":[{"b64_json":"aGVsbG8="}],`+
			`"usage":{"total_tokens":15,"input_tokens":12,"output_tokens":3}}`)
	case "empty_data":
		fmt.Fprint(w, `{"created":1720000000,"data":[]}`)
	case "bad_shape":
		fmt.Fprint(w, `{"created":1720000000,"data":[{"revised_prompt":"darker square"}]}`)
	default:
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprint(w, `{"error":{"message":"No available providers for this model"}}`)
	}
}

// createHubViaAPI registers a hub pointing at hubURL and returns its ID.
func createHubViaAPI(t *testing.T, baseURL, hubURL string) int {
	t.Helper()
	resp := doPost(t, baseURL+"/api/hubs", map[string]interface{}{
		"name":     "hub",
		"base_url": hubURL,
		"token":    "test-token-0000",
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create hub: expected 201, got %d", resp.StatusCode)
	}
	var env envelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode hub: %v", err)
	}
	var hub map[string]interface{}
	if err := json.Unmarshal(env.Data, &hub); err != nil {
		t.Fatalf("unmarshal hub: %v", err)
	}
	return int(hub["id"].(float64))
}

// runDiscovery triggers POST /api/discovery/run and returns the stats object.
func runDiscovery(t *testing.T, baseURL string) map[string]interface{} {
	t.Helper()
	resp := doPost(t, baseURL+"/api/discovery/run", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("discovery run: expected 200, got %d: %s", resp.StatusCode, body)
	}
	var env envelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode discovery stats: %v", err)
	}
	var stats map[string]interface{}
	if err := json.Unmarshal(env.Data, &stats); err != nil {
		t.Fatalf("unmarshal discovery stats: %v", err)
	}
	return stats
}

// listModelsViaAPI fetches GET /api/models keyed by model_id.
func listModelsViaAPI(t *testing.T, baseURL string) map[string]map[string]interface{} {
	t.Helper()
	resp := doGet(t, baseURL+"/api/models")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list models: expected 200, got %d", resp.StatusCode)
	}
	var env envelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode models: %v", err)
	}
	var models []map[string]interface{}
	if err := json.Unmarshal(env.Data, &models); err != nil {
		t.Fatalf("unmarshal models: %v", err)
	}
	byID := make(map[string]map[string]interface{}, len(models))
	for _, m := range models {
		byID[m["model_id"].(string)] = m
	}
	return byID
}

// hasEndpoint reports whether the model has an endpoint for the protocol.
func hasEndpoint(model map[string]interface{}, protocol string) bool {
	for _, e := range model["endpoints"].([]interface{}) {
		if e.(map[string]interface{})["protocol"].(string) == protocol {
			return true
		}
	}
	return false
}

// endpointEnabled returns the enabled flag of the model's endpoint for the
// given protocol.
func endpointEnabled(t *testing.T, model map[string]interface{}, protocol string) bool {
	t.Helper()
	for _, e := range model["endpoints"].([]interface{}) {
		ep := e.(map[string]interface{})
		if ep["protocol"].(string) == protocol {
			return ep["enabled"].(bool)
		}
	}
	t.Fatalf("model %v has no %s endpoint", model["model_id"], protocol)
	return false
}

// statNumber reads a numeric discovery stat.
func statNumber(t *testing.T, stats map[string]interface{}, key string) int {
	t.Helper()
	v, ok := stats[key].(float64)
	if !ok {
		t.Fatalf("stats missing numeric key %q: %v", key, stats)
	}
	return int(v)
}

func TestDiscoveryFirstSync(t *testing.T) {
	db := openTempDB(t)
	ts := newTestAPIServer(t, db)

	stub := newDiscoveryStubHub(t, []string{"gpt-5", "gpt-image-2"})
	// gpt-5 speaks openai only: anthropic probes must fail.
	stub.setFailing("gpt-5", "anthropic")
	hubID := createHubViaAPI(t, ts.URL, stub.URL)

	// Hub creation triggers an asynchronous first sync; wait for it to finish.
	waitForHubSyncStatus(t, ts.URL, hubID, "succeeded")

	// The model list call must authenticate like an openai-protocol request.
	if auth := stub.listAuth(); auth != "Bearer test-token-0000" {
		t.Errorf("/v1/models Authorization: expected Bearer header, got %q", auth)
	}

	models := listModelsViaAPI(t, ts.URL)
	if len(models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(models))
	}

	gpt5 := models["gpt-5"]
	if gpt5["origin"] != "discovered" {
		t.Errorf("gpt-5 origin: expected discovered, got %v", gpt5["origin"])
	}
	if gpt5["status"] != "active" {
		t.Errorf("gpt-5 status: expected active, got %v", gpt5["status"])
	}
	if gpt5["capability"] != "chat" {
		t.Errorf("gpt-5 capability: expected chat, got %v", gpt5["capability"])
	}
	// Only the working protocol gets an endpoint; the failed trial leaves
	// none at all (ticket 17).
	if !hasEndpoint(gpt5, "openai") {
		t.Error("gpt-5 should have an openai endpoint (probe succeeded)")
	}
	if hasEndpoint(gpt5, "anthropic") {
		t.Error("gpt-5 should have NO anthropic endpoint (trial failed)")
	}
	if endpointEnabled(t, gpt5, "openai") != true {
		t.Error("gpt-5 openai endpoint should be enabled")
	}

	img := models["gpt-image-2"]
	if img["capability"] != "image" {
		t.Errorf("gpt-image-2 capability: expected image, got %v", img["capability"])
	}
	if !hasEndpoint(img, "openai") || !hasEndpoint(img, "anthropic") {
		t.Error("gpt-image-2 should have both endpoints (both trials succeeded)")
	}

	// A full sync over the unchanged list must be a no-op.
	stats := runDiscovery(t, ts.URL)
	if got := statNumber(t, stats, "added"); got != 0 {
		t.Errorf("second sync added: expected 0, got %d", got)
	}
	if got := statNumber(t, stats, "endpoints_created"); got != 0 {
		t.Errorf("second sync endpoints_created: expected 0, got %d", got)
	}
}

func TestDiscoveryRetireAndRestore(t *testing.T) {
	db := openTempDB(t)
	ts := newTestAPIServer(t, db)

	stub := newDiscoveryStubHub(t, []string{"model-a", "model-b"})
	hubID := createHubViaAPI(t, ts.URL, stub.URL)
	waitForHubSyncStatus(t, ts.URL, hubID, "succeeded")

	// model-b vanishes from the hub listing.
	stub.setModels([]string{"model-a"})
	stats := runDiscovery(t, ts.URL)
	if got := statNumber(t, stats, "retired"); got != 1 {
		t.Errorf("retired: expected 1, got %d", got)
	}

	models := listModelsViaAPI(t, ts.URL)
	if status := models["model-b"]["status"]; status != "retired" {
		t.Errorf("model-b status: expected retired, got %v", status)
	}
	if status := models["model-a"]["status"]; status != "active" {
		t.Errorf("model-a status: expected active, got %v", status)
	}

	// model-b reappears: it is restored, not double-created.
	stub.setModels([]string{"model-a", "model-b"})
	stats = runDiscovery(t, ts.URL)
	if got := statNumber(t, stats, "added"); got != 0 {
		t.Errorf("restore sync added: expected 0, got %d", got)
	}
	if got := statNumber(t, stats, "retired"); got != 0 {
		t.Errorf("restore sync retired: expected 0, got %d", got)
	}

	models = listModelsViaAPI(t, ts.URL)
	if status := models["model-b"]["status"]; status != "active" {
		t.Errorf("model-b status after restore: expected active, got %v", status)
	}
	if endpoints := models["model-b"]["endpoints"].([]interface{}); len(endpoints) != 2 {
		t.Errorf("model-b should keep its original 2 endpoints, got %d", len(endpoints))
	}
}

func TestDiscoveryNeverRetiresManualModels(t *testing.T) {
	db := openTempDB(t)
	ts := newTestAPIServer(t, db)

	stub := newDiscoveryStubHub(t, []string{"auto-model"})
	hubID := createHubViaAPI(t, ts.URL, stub.URL)

	// The auto-sync triggered by creation registers auto-model already.
	waitForHubSyncStatus(t, ts.URL, hubID, "succeeded")

	// A manually registered model outside the hub listing (e.g. a [1M] variant).
	resp := doPost(t, ts.URL+"/api/models", map[string]interface{}{
		"hub_id":   hubID,
		"model_id": "custom-1m-variant",
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create manual model: expected 201, got %d", resp.StatusCode)
	}

	// A full sync over the unchanged listing is a no-op; the manual model is
	// untouched even though it is absent from the hub listing.
	stats := runDiscovery(t, ts.URL)
	if got := statNumber(t, stats, "added"); got != 0 {
		t.Errorf("added: expected 0 (auto-model already registered), got %d", got)
	}
	if got := statNumber(t, stats, "retired"); got != 0 {
		t.Errorf("retired: expected 0 (manual models never retire), got %d", got)
	}

	// The hub listing shrinks: only the discovered model may retire.
	stub.setModels([]string{"some-other-model"})
	stats = runDiscovery(t, ts.URL)
	if got := statNumber(t, stats, "retired"); got != 1 {
		t.Errorf("retired: expected 1 (auto-model only), got %d", got)
	}

	models := listModelsViaAPI(t, ts.URL)
	if status := models["auto-model"]["status"]; status != "retired" {
		t.Errorf("auto-model status: expected retired, got %v", status)
	}
	manual := models["custom-1m-variant"]
	if status := manual["status"]; status != "active" {
		t.Errorf("manual model status: expected active, got %v", status)
	}
	if origin := manual["origin"]; origin != "manual" {
		t.Errorf("manual model origin: expected manual, got %v", origin)
	}
}

// TestDiscoveryEmptyListRetiresNothing guards against a hub anomaly that
// returns 200 with an empty model list: nothing may be retired in that case.
func TestDiscoveryEmptyListRetiresNothing(t *testing.T) {
	db := openTempDB(t)
	ts := newTestAPIServer(t, db)

	stub := newDiscoveryStubHub(t, []string{"auto-model"})
	hubID := createHubViaAPI(t, ts.URL, stub.URL)
	waitForHubSyncStatus(t, ts.URL, hubID, "succeeded")

	stub.setModels(nil)
	stats := runDiscovery(t, ts.URL)
	if got := statNumber(t, stats, "retired"); got != 0 {
		t.Errorf("retired: expected 0 (empty list anomaly guard), got %d", got)
	}
	models := listModelsViaAPI(t, ts.URL)
	if status := models["auto-model"]["status"]; status != "active" {
		t.Errorf("auto-model status: expected active, got %v", status)
	}
}
