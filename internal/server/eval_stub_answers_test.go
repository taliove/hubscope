package server_test

import "strings"

// answerFor decides the response text by model and prompt content.
//
// Post-cutover (ticket 99) the seeded bank is the five authoritative-benchmark
// suites; tests that need specific scores script them per prompt via
// setAnswerSeq (benchmark tests, normalization stages) or install custom
// exact-rule cases whose expectation is the default answer "好的" — so the
// stub needs no per-seed-case answer table anymore. Judge calls are
// recognized by the judge prompt marker (the judge model name is configurable
// via settings), returning a valid JSON verdict; a scripted judge sequence
// wins over the default verdict so sampling tests can mix scored and failed
// samples.
func (h *evalStubHub) answerFor(model, prompt string) string {
	if strings.Contains(prompt, "你是评估裁判") {
		if text, ok := h.nextSeq(h.judgeSeq, prompt); ok {
			return text
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
	return "好的"
}
