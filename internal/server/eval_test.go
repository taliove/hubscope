package server_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/taliove2009/hubscope/internal/server"
	"github.com/taliove2009/hubscope/internal/store"
)

// evalStubHub simulates a Hub for eval tests: it answers by prompt content
// (correctly for "smart" models, wrongly for "dumb-model"), fails hard for
// "broken-model", and role-plays the LLM judge — including a garbage-verdict
// branch for one specific case prompt. Models registered via markBad flip
// from correct to wrong answers, which drives score-drop scenarios.
// setAnswerSeq/setJudgeSeq script per-call response sequences (cycled) for
// prompts containing a marker, which drives sampling-average scenarios.
type evalStubHub struct {
	*httptest.Server
	mu sync.Mutex
	// calls records which protocols each model was called with.
	calls map[string]map[string]bool
	// callCounts records how many completion calls carried (model, prompt).
	callCounts map[string]int
	// bad marks models that currently answer everything wrong.
	bad map[string]bool
	// broken marks models whose calls fail with HTTP 503.
	broken map[string]bool
	// gate, when non-nil, blocks every response until released; tests use it
	// to freeze a run mid-flight (e.g. to cancel its context deterministically).
	gate chan struct{}
	// totalCalls records how many completion calls each model made (any
	// prompt), the counter blockModelAfter thresholds compare against.
	totalCalls map[string]int
	// gateAfter, when set for a model, blocks the model's responses once its
	// recorded completion-call count passes the threshold, so a test can
	// freeze a model after exactly n calls completed (ticket 52 progress-grid
	// scenarios). The gate channel lives in modelGates.
	gateAfter map[string]int
	// modelGates holds the block/release channels of gated models.
	modelGates map[string]chan struct{}
	// answerSeq scripts cycled answer responses by prompt marker.
	answerSeq map[string][]string
	// judgeSeq scripts cycled judge responses by prompt marker.
	judgeSeq map[string][]string
}

func newEvalStubHub() *evalStubHub {
	stub := &evalStubHub{
		calls:      map[string]map[string]bool{},
		callCounts: map[string]int{},
		bad:        map[string]bool{},
		broken:     map[string]bool{},
		answerSeq:  map[string][]string{},
		judgeSeq:   map[string][]string{},
		totalCalls: map[string]int{},
		gateAfter:  map[string]int{},
		modelGates: map[string]chan struct{}{},
	}
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
		Stream   bool   `json:"stream"`
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
	h.callCounts[req.Model+"\x00"+prompt]++
	h.totalCalls[req.Model]++
	calls := h.totalCalls[req.Model]
	h.mu.Unlock()

	h.mu.Lock()
	broken := h.broken[req.Model]
	gate := h.gate
	modelGate := h.modelGates[req.Model]
	if limit, gated := h.gateAfter[req.Model]; gated && calls <= limit {
		modelGate = nil // still within the allowed-call budget
	}
	h.mu.Unlock()
	// A closed gate holds the response until the test releases it; the call
	// above is already recorded, so waiters observe the call while blocked.
	if gate != nil {
		<-gate
	}
	if modelGate != nil {
		<-modelGate
	}
	if broken {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]string{"message": "No available providers for this model"},
		})
		return
	}

	answer := h.answerFor(req.Model, prompt)
	if req.Stream {
		h.writeStream(w, isAnthropic, answer)
		return
	}
	h.writeCompletion(w, isAnthropic, answer)
}

// writeStream answers a streaming request with a well-formed SSE reply
// carrying the given text, so probe rounds against this stub exercise both
// probe modes. The evaluator itself never streams.
func (h *evalStubHub) writeStream(w http.ResponseWriter, isAnthropic bool, text string) {
	w.Header().Set("Content-Type", "text/event-stream")
	flusher, _ := w.(http.Flusher)
	flush := func() {
		if flusher != nil {
			flusher.Flush()
		}
	}
	if isAnthropic {
		for _, event := range []string{
			`{"type":"message_start","message":{"id":"msg_eval","type":"message","role":"assistant","content":[],"usage":{"input_tokens":12,"output_tokens":0}}}`,
			`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
			fmt.Sprintf(`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":%s}}`, strconv.Quote(text)),
			`{"type":"content_block_stop","index":0}`,
			`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":8}}`,
			`{"type":"message_stop"}`,
		} {
			fmt.Fprintf(w, "event: message\ndata: %s\n\n", event)
			flush()
		}
		return
	}
	for _, chunk := range []string{
		`{"id":"chatcmpl_eval","object":"chat.completion.chunk","choices":[{"delta":{"role":"assistant"},"index":0}]}`,
		fmt.Sprintf(`{"id":"chatcmpl_eval","object":"chat.completion.chunk","choices":[{"delta":{"content":%s},"index":0}]}`, strconv.Quote(text)),
		`{"id":"chatcmpl_eval","object":"chat.completion.chunk","choices":[{"delta":{},"index":0,"finish_reason":"stop"}],"usage":{"prompt_tokens":12,"completion_tokens":8}}`,
	} {
		fmt.Fprintf(w, "data: %s\n\n", chunk)
		flush()
	}
	fmt.Fprint(w, "data: [DONE]\n\n")
	flush()
}

// answerFor decides the response text by model and prompt content.
func (h *evalStubHub) answerFor(model, prompt string) string {
	// Judge calls are recognized by the裁判 prompt marker (the judge model
	// name is configurable via settings), returning a valid JSON verdict —
	// except for the formal-rewrite case whose embedded prompt marker
	// triggers an unparseable reply. A scripted judge sequence wins over the
	// default verdict so sampling tests can mix scored and failed samples.
	if strings.Contains(prompt, "你是评估裁判") {
		if text, ok := h.nextSeq(h.judgeSeq, prompt); ok {
			return text
		}
		if strings.Contains(prompt, "改写成更正式") {
			return "I cannot produce a score for this."
		}
		return `{"score": 0.75, "reason": "meets the rubric"}`
	}
	if text, ok := h.nextSeq(h.answerSeq, prompt); ok {
		return text
	}
	h.mu.Lock()
	bad := h.bad[model]
	h.mu.Unlock()
	if bad || model == "dumb-model" {
		return "随便说点什么"
	}
	// Smart models answer every seed case correctly.
	switch {
	// Question-bank v3 (gen 3) seed answers. Markers are unique substrings of
	// the v3 prompts; none of the v3 prompts contain a legacy marker, so the
	// two banks never cross-match.
	// cap_instruction
	case strings.Contains(prompt, "张伟去年从上海"):
		return `{"name": "张伟", "city": "杭州"}`
	case strings.Contains(prompt, "桌子上放着苹果"):
		return "3"
	case strings.Contains(prompt, "the hub is healthy"):
		return "THE HUB IS HEALTHY"
	case strings.Contains(prompt, "客户订购了 2 台笔记本电脑"):
		return "笔记本电脑x2,无线鼠标x5"
	case strings.Contains(prompt, "离太阳最近的两颗行星"):
		return "| 排名 | 行星 |\n| --- | --- |\n| 1 | 水星 |\n| 2 | 金星 |"
	case strings.Contains(prompt, "年份：2100"):
		return "平年"
	case strings.Contains(prompt, "季度预算评审会定于"):
		return `{"month": 3, "day": 15, "room": 301}`
	case strings.Contains(prompt, "banana apple cherry"):
		return "apple|banana|cherry"
	case strings.Contains(prompt, "字母表中的后一个字母"):
		return "bcd"
	case strings.Contains(prompt, "人工智能正在改变世界"):
		return "10"
	// cap_reasoning
	case strings.Contains(prompt, "3 盒铅笔"):
		return "31"
	case strings.Contains(prompt, "长 8 厘米"):
		return "26"
	case strings.Contains(prompt, "100 小时后"):
		return "19"
	case strings.Contains(prompt, "火车以每秒 20 米"):
		return "30"
	case strings.Contains(prompt, "男生比女生多 6 人"):
		return "24"
	case strings.Contains(prompt, "三个连续偶数"):
		return "18"
	case strings.Contains(prompt, "相距 60 千米"):
		return "4"
	case strings.Contains(prompt, "单开进水管 6 小时"):
		return "15"
	case strings.Contains(prompt, "3 倍等于另一部分"):
		return "40"
	case strings.Contains(prompt, "咪咪是猫"):
		return "是"
	// cap_coding
	case strings.Contains(prompt, `len("hubscope")`):
		return "8"
	case strings.Contains(prompt, "7 // 2"):
		return "3"
	case strings.Contains(prompt, "typeof null"):
		return "object"
	case strings.Contains(prompt, "x * 2 for x in range(3)"):
		return "[0, 2, 4]"
	case strings.Contains(prompt, `"abc".upper()`):
		return "BC"
	case strings.Contains(prompt, `"5" + 3`):
		return "53"
	case strings.Contains(prompt, "print(f(3, 3))"):
		return "27"
	case strings.Contains(prompt, `int("abc")`):
		return "ValueError"
	case strings.Contains(prompt, "[]int{1, 2, 3, 4}"):
		return "2"
	case strings.Contains(prompt, "sum(d.values())"):
		return "3"
	// cap_knowledge (each returns the correct option letter)
	case strings.Contains(prompt, "水的化学式"):
		return "B"
	case strings.Contains(prompt, "首都是哪座城市"):
		return "C"
	case strings.Contains(prompt, "三角形内角和"):
		return "B"
	case strings.Contains(prompt, "光在真空中"):
		return "B"
	case strings.Contains(prompt, "红楼梦"):
		return "C"
	case strings.Contains(prompt, "表面积最大的器官"):
		return "B"
	case strings.Contains(prompt, "TCP 协议"):
		return "C"
	case strings.Contains(prompt, "床前明月光"):
		return "A"
	case strings.Contains(prompt, "面积最大的海洋"):
		return "C"
	case strings.Contains(prompt, "多少个节气"):
		return "B"
	// cap_language (rule cases; judge cases fall through to the default answer
	// and are scored by the stub judge)
	case strings.Contains(prompt, "差一点没摔倒"):
		return "没有摔倒"
	case strings.Contains(prompt, "难以下咽"):
		return "不满"
	case strings.Contains(prompt, "大败美国队"):
		return "中国队"
	case strings.Contains(prompt, "我、昨天、公园"):
		return "我昨天去了公园"
	case strings.Contains(prompt, "不得不离开"):
		return "B"
	case strings.Contains(prompt, "咬死了猎人的狗"):
		return "2"
	case strings.Contains(prompt, "pong"):
		return "pong"
	case strings.Contains(prompt, "严格的 JSON"):
		return `{"ok": true}`
	case strings.Contains(prompt, "数到 3"):
		return "1\n2\n3"
	case strings.Contains(prompt, "不要任何标点"):
		return "hello"
	case strings.Contains(prompt, "翻译成英文"):
		return "artificial intelligence"
	case strings.Contains(prompt, "什么是递归"):
		return "递归就是函数调用自己来解决问题的编程技巧"
	case strings.Contains(prompt, "中文大写"):
		return "四十二"
	case strings.Contains(prompt, "重复我说的话"):
		return "天气真好"
	case strings.Contains(prompt, "所有偶数"):
		return "2,4"
	case strings.Contains(prompt, "首字母大写"):
		return "Hello World"
	case strings.Contains(prompt, "3+4*2"):
		return "11"
	case strings.Contains(prompt, "「abcdef」"):
		return "fedcba"
	case strings.Contains(prompt, "17 + 25"):
		return "42"
	case strings.Contains(prompt, "游泳"):
		return "3"
	case strings.Contains(prompt, "下一个数字"):
		return "13"
	case strings.Contains(prompt, "最大的质数"):
		return "97"
	case strings.Contains(prompt, "7 的平方"):
		return "49"
	case strings.Contains(prompt, "乘以 3 等于 51"):
		return "17"
	case strings.Contains(prompt, "鸡兔同笼"):
		return "6"
	case strings.Contains(prompt, "一本书 120 页"):
		return "90"
	case strings.Contains(prompt, "等差数列"):
		return "29"
	case strings.Contains(prompt, "涨价 10%"):
		return "99"
	case strings.Contains(prompt, "log2(64)"):
		return "6"
	case strings.Contains(prompt, "有多少种选法"):
		return "10"
	case strings.Contains(prompt, "add(a, b)"):
		return "def add(a, b):\n    return a + b"
	case strings.Contains(prompt, "len([1,2,3])"):
		return "6"
	case strings.Contains(prompt, "'hello'[1]"):
		return "e"
	case strings.Contains(prompt, "2 ** 10"):
		return "1024"
	case strings.Contains(prompt, "'ab' * 3"):
		return "ababab"
	case strings.Contains(prompt, "is_even"):
		return "def is_even(n):\n    return n % 2 == 0"
	case strings.Contains(prompt, "map(x => x * 2)"):
		return "3"
	case strings.Contains(prompt, "sorted([3,1,2])"):
		return "1"
	case strings.Contains(prompt, "join(['a','b','c'])"):
		return "a,b,c"
	case strings.Contains(prompt, "reverse_string"):
		return "def reverse_string(s):\n    return s[::-1]"
	case strings.Contains(prompt, "list(range(5))"):
		return "10"
	case strings.Contains(prompt, "{'a':1,'b':2}"):
		return "2"
	default:
		return "好的"
	}
}

// nextSeq pops the next scripted response for the first marker contained in
// prompt, cycling through the sequence. The second return value reports
// whether a script matched.
func (h *evalStubHub) nextSeq(seqs map[string][]string, prompt string) (string, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for marker, seq := range seqs {
		if !strings.Contains(prompt, marker) || len(seq) == 0 {
			continue
		}
		text := seq[0]
		seqs[marker] = append(seq[1:], seq[0])
		return text, true
	}
	return "", false
}

// setAnswerSeq scripts cycled answer responses for prompts containing marker.
// setJudgeSeq does the same for judge calls (prompts carrying the裁判 marker).
func (h *evalStubHub) setAnswerSeq(marker string, seq ...string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.answerSeq[marker] = seq
}

func (h *evalStubHub) setJudgeSeq(marker string, seq ...string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.judgeSeq[marker] = seq
}

// callCount reports how many completion calls carried exactly the given
// prompt for the model (one per sample).
func (h *evalStubHub) callCount(model, prompt string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.callCounts[model+"\x00"+prompt]
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
	h.totalCalls = map[string]int{}
}

func (h *evalStubHub) markBad(model string, bad bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.bad[model] = bad
}

// blockCalls makes every subsequent completion response wait until release;
// calls in flight keep being recorded so tests can observe them while blocked.
func (h *evalStubHub) blockCalls() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.gate = make(chan struct{})
}

// release unblocks all gated responses and reopens normal answering.
func (h *evalStubHub) release() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.gate != nil {
		close(h.gate)
		h.gate = nil
	}
}

// blockModelAfter freezes the given model's responses once its recorded
// completion-call count passes n — the first n calls answer normally, every
// later call waits until releaseModel. blockModel freezes from the first
// call on (n = 0).
func (h *evalStubHub) blockModelAfter(model string, n int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.gateAfter[model] = n
	h.modelGates[model] = make(chan struct{})
}

func (h *evalStubHub) blockModel(model string) {
	h.blockModelAfter(model, 0)
}

// releaseModel unblocks the given model's gated responses.
func (h *evalStubHub) releaseModel(model string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if gate, ok := h.modelGates[model]; ok {
		close(gate)
		delete(h.modelGates, model)
		delete(h.gateAfter, model)
	}
}

// callTotal reports how many completion calls the model made in total (any
// prompt), including calls currently blocked on a gate.
func (h *evalStubHub) callTotal(model string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.totalCalls[model]
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
