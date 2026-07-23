package server_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
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
