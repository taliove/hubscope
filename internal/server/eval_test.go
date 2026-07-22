package server_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"git.github.net/taliove2009/ai-hub-checker/internal/server"
	"git.github.net/taliove2009/ai-hub-checker/internal/store"
)

// evalStubHub simulates a Hub for eval tests: it answers by prompt content
// (correctly for "smart" models, wrongly for "dumb-model"), fails hard for
// "broken-model", and role-plays the LLM judge — including a garbage-verdict
// branch for one specific case prompt. Models registered via markBad flip
// from correct to wrong answers, which drives score-drop scenarios.
type evalStubHub struct {
	*httptest.Server
	mu sync.Mutex
	// calls records which protocols each model was called with.
	calls map[string]map[string]bool
	// bad marks models that currently answer everything wrong.
	bad map[string]bool
	// broken marks models whose calls fail with HTTP 503.
	broken map[string]bool
}

func newEvalStubHub() *evalStubHub {
	stub := &evalStubHub{calls: map[string]map[string]bool{}, bad: map[string]bool{}, broken: map[string]bool{}}
	stub.Server = httptest.NewServer(http.HandlerFunc(stub.handle))
	return stub
}

func (h *evalStubHub) handle(w http.ResponseWriter, r *http.Request) {
	isAnthropic := strings.HasSuffix(r.URL.Path, "/v1/messages")
	isOpenAI := strings.HasSuffix(r.URL.Path, "/v1/chat/completions")
	if !isAnthropic && !isOpenAI {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	protocol := "openai"
	if isAnthropic {
		protocol = "anthropic"
	}

	body, _ := io.ReadAll(r.Body)
	var req struct {
		Model    string `json:"model"`
		Messages []struct {
			Content string `json:"content"`
		} `json:"messages"`
	}
	_ = json.Unmarshal(body, &req)
	prompt := ""
	if len(req.Messages) > 0 {
		prompt = req.Messages[0].Content
	}

	h.mu.Lock()
	if h.calls[req.Model] == nil {
		h.calls[req.Model] = map[string]bool{}
	}
	h.calls[req.Model][protocol] = true
	h.mu.Unlock()

	h.mu.Lock()
	broken := h.broken[req.Model]
	h.mu.Unlock()
	if broken {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]string{"message": "No available providers for this model"},
		})
		return
	}

	h.writeCompletion(w, isAnthropic, h.answerFor(req.Model, prompt))
}

// answerFor decides the response text by model and prompt content.
func (h *evalStubHub) answerFor(model, prompt string) string {
	// Judge calls are recognized by the裁判 prompt marker (the judge model
	// name is configurable via settings), returning a valid JSON verdict —
	// except for the formal-rewrite case whose embedded prompt marker
	// triggers an unparseable reply.
	if strings.Contains(prompt, "你是评估裁判") {
		if strings.Contains(prompt, "改写成更正式") {
			return "I cannot produce a score for this."
		}
		return `{"score": 0.75, "reason": "meets the rubric"}`
	}
	h.mu.Lock()
	bad := h.bad[model]
	h.mu.Unlock()
	if bad || model == "dumb-model" {
		return "随便说点什么"
	}
	// Smart models answer every seed case correctly.
	switch {
	case strings.Contains(prompt, "pong"):
		return "pong"
	case strings.Contains(prompt, "严格的 JSON"):
		return `{"ok": true}`
	case strings.Contains(prompt, "数到 3"):
		return "1\n2\n3"
	case strings.Contains(prompt, "17 + 25"):
		return "42"
	case strings.Contains(prompt, "游泳"):
		return "3"
	case strings.Contains(prompt, "下一个数字"):
		return "13"
	case strings.Contains(prompt, "add(a, b)"):
		return "def add(a, b):\n    return a + b"
	case strings.Contains(prompt, "len([1,2,3])"):
		return "6"
	case strings.Contains(prompt, "'hello'[1]"):
		return "e"
	default:
		return "好的"
	}
}

func (h *evalStubHub) writeCompletion(w http.ResponseWriter, isAnthropic bool, text string) {
	w.Header().Set("Content-Type", "application/json")
	if isAnthropic {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id":      "msg_eval",
			"type":    "message",
			"role":    "assistant",
			"content": []map[string]string{{"type": "text", "text": text}},
			"usage":   map[string]int{"input_tokens": 12, "output_tokens": 8},
		})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"id":     "chatcmpl_eval",
		"object": "chat.completion",
		"choices": []map[string]interface{}{
			{"message": map[string]string{"role": "assistant", "content": text}, "finish_reason": "stop"},
		},
		"usage": map[string]int{"prompt_tokens": 12, "completion_tokens": 8},
	})
}

// sawProtocol reports whether the model was called over the given protocol.
func (h *evalStubHub) sawProtocol(model, protocol string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.calls[model][protocol]
}

// sawModel reports whether any completion call carried the given model name.
func (h *evalStubHub) sawModel(model string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.calls[model]) > 0
}

// markBad flips a model between correct (false) and always-wrong (true)
// answers, so tests can move its eval scores between rounds.
// markBroken makes the model's calls fail with HTTP 503 (or recover).
func (h *evalStubHub) markBroken(model string, broken bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.broken[model] = broken
}

// resetCalls clears the protocol call log, so assertions only see calls
// made after this point (creation trial probes pollute it otherwise).
func (h *evalStubHub) resetCalls() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.calls = map[string]map[string]bool{}
}

func (h *evalStubHub) markBad(model string, bad bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.bad[model] = bad
}

// setupEvalEnv builds an isolated API server + stub Hub + real SQLite DB.
func setupEvalEnv(t *testing.T) (*httptest.Server, *evalStubHub, *store.DB) {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	stub := newEvalStubHub()
	ts := httptest.NewServer(server.New(db, testAdminPassword, server.WithRateLimits(server.RateLimits{})))
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

// triggerEval starts an eval run and returns its ID; expects HTTP 202.
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
	var run map[string]interface{}
	_ = json.Unmarshal(env.Data, &run)
	if run["status"] != "running" {
		t.Errorf("new run status = %v, want running", run["status"])
	}
	if run["trigger"] != "manual" {
		t.Errorf("new run trigger = %v, want manual", run["trigger"])
	}
	if run["judge_model"] != store.DefaultJudgeModel {
		t.Errorf("judge_model = %v, want %s", run["judge_model"], store.DefaultJudgeModel)
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
	if len(smart) != 3 {
		t.Fatalf("smart model has %d results, want 3", len(smart))
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
	if len(dumb) != 3 {
		t.Fatalf("dumb model has %d results, want 3", len(dumb))
	}
	for _, r := range dumb {
		if r["score"] != 0.0 {
			t.Errorf("dumb case %v score = %v, want 0 (detail: %v)", r["case_id"], r["score"], r["verdict_detail"])
		}
	}

	// Aggregate: (3 x 1 + 3 x 0) / 6 = 0.5.
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
