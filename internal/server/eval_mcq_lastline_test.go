package server_test

import (
	"encoding/json"
	"math"
	"net/http"
	"strings"
	"testing"
)

// Black-box coverage for the mcq last-line extraction fallback (field report
// 2026-08-03): a model whose long reasoning ends with a lone option letter
// on the final line was misjudged "no unambiguous option letter extracted"
// and scored 0 despite answering correctly. The fallback must rescue exactly
// that shape — and nothing more: word-internal letters (长征5号B), missing
// letters, and multi-letter or non-letter final lines all stay
// unextractable (ADR 0013 宁缺毋猜). Zero judge calls are involved.
//
// The cases are installed over the mmlu suite (seeded cases retired) so the
// run keeps the four-option nadir; with an even 2-hit/3-miss split the
// nadir-normalized aggregate is (0.4 - 0.25) / 0.75 = 0.2.

// createMCQCase posts one mcq rule case to the suite, asserts HTTP 201 and
// returns the created case's ID.
func createMCQCase(t *testing.T, base string, suiteID int64, prompt, expected string) int64 {
	t.Helper()
	resp := doPost(t, base+"/api/cases", map[string]interface{}{
		"suite_id":     suiteID,
		"prompt":       prompt,
		"verdict_type": "rule",
		"rule_config":  map[string]interface{}{"mode": "mcq", "expected": expected},
	})
	if resp.StatusCode != http.StatusCreated {
		resp.Body.Close()
		t.Fatalf("create mcq case %q: expected 201, got %d", prompt, resp.StatusCode)
	}
	var env envelope
	_ = json.NewDecoder(resp.Body).Decode(&env)
	resp.Body.Close()
	var created map[string]interface{}
	_ = json.Unmarshal(env.Data, &created)
	return int64(created["id"].(float64))
}

// mcqFallbackScenario is one case of the extraction-fallback matrix: the
// scripted model answer, the score it must receive, and whether the verdict
// detail must report a match or an extraction failure.
type mcqFallbackScenario struct {
	marker   string // unique prompt substring the stub keys the answer on
	prompt   string
	expected string
	answer   string
	want     float64
	matched  bool // true: "rule mcq matched"; false: "no unambiguous option letter"
	caseID   int64
}

func TestMCQLastLineFallback(t *testing.T) {
	ts, stub, db := setupEvalEnv(t)
	modelID := createEvalModel(t, ts.URL, stub.URL, "mcq-lastline")
	suiteID := suiteIDByKey(t, ts.URL, "mmlu")
	retireSuiteCases(t, db, suiteID)

	// Shared option block: the markers let the verdict infer the candidate
	// letter set from the question instead of a hardcoded A-D.
	const options = "\nA. 甲\nB. 乙\nC. 丙\nD. 丁"
	scenarios := []mcqFallbackScenario{
		{
			// The field report itself: a long timezone-conversion
			// reasoning (with the word-internal letter of 长征5号B as
			// noise) committing to a lone "C" on the final line.
			marker:   "TZCASE",
			prompt:   "TZCASE: 北京时间 08:00 对应纽约(夏令时)几点?" + options,
			expected: "C",
			answer: "先算时区差:北京为 UTC+8,纽约夏令时为 UTC-4,相差 12 小时。\n" +
				"08:00 减去 12 小时,落在前一日的 20:00。\n" +
				"核对干扰项:长征5号B运载火箭与本题无关,只是文本里的词内字母。\n" +
				"C",
			want:    1,
			matched: true,
		},
		{
			// The fallback tolerates one trailing punctuation mark.
			marker:   "PUNCTCASE",
			prompt:   "PUNCTCASE: 哪项正确?" + options,
			expected: "C",
			answer:   "逐项排除后只剩一个选项成立。\nC。",
			want:     1,
			matched:  true,
		},
		{
			// Word-internal letters never qualify: 长征5号B mentions B,
			// the expected letter, yet with no committed option line the
			// answer must stay unextractable.
			marker:   "WORDCASE",
			prompt:   "WORDCASE: 哪型火箭执行空间站舱段发射?" + options,
			expected: "B",
			answer:   "长征5号B运载火箭在文昌航天发射场首飞成功,近地轨道运力见公开报道。",
			want:     0,
			matched:  false,
		},
		{
			// No letter anywhere: nothing to extract.
			marker:   "NOCASE",
			prompt:   "NOCASE: 哪项正确?" + options,
			expected: "A",
			answer:   "这个问题我需要更多上下文,暂时给不出确定的结论。",
			want:     0,
			matched:  false,
		},
		{
			// A multi-letter final line is not a lone commitment.
			marker:   "MULTICASE",
			prompt:   "MULTICASE: 哪项正确?" + options,
			expected: "C",
			answer:   "两个选项看起来都成立,我无法取舍。\nC和D",
			want:     0,
			matched:  false,
		},
	}
	for i := range scenarios {
		scenarios[i].caseID = createMCQCase(t, ts.URL, suiteID, scenarios[i].prompt, scenarios[i].expected)
		stub.setAnswerSeq(scenarios[i].marker, scenarios[i].answer)
	}

	runID := triggerEval(t, ts.URL, suiteID, modelID)
	run := waitEvalDone(t, ts.URL, runID)
	if run["status"] != "done" {
		t.Fatalf("run status = %v, want done", run["status"])
	}

	results := resultsByModel(run, "mcq-lastline")
	if len(results) != len(scenarios) {
		t.Fatalf("got %d results, want %d", len(results), len(scenarios))
	}
	byCaseID := map[int64]map[string]interface{}{}
	for _, r := range results {
		byCaseID[int64(r["case_id"].(float64))] = r
	}
	for _, sc := range scenarios {
		r, ok := byCaseID[sc.caseID]
		if !ok {
			t.Fatalf("result for %s missing", sc.marker)
		}
		if r["score"] != sc.want {
			t.Errorf("%s score = %v, want %v (answer %q, detail: %v)",
				sc.marker, r["score"], sc.want, sc.answer, r["verdict_detail"])
		}
		detail := r["verdict_detail"].(string)
		if sc.matched {
			if !strings.Contains(detail, "rule mcq matched") {
				t.Errorf("%s verdict_detail should explain the mcq match: %v", sc.marker, detail)
			}
		} else if !strings.Contains(detail, "no unambiguous option letter") {
			t.Errorf("%s verdict_detail should report the extraction failure: %v", sc.marker, detail)
		}
		if strings.Contains(detail, "judge") {
			t.Errorf("%s rule verdict must never involve the judge: %v", sc.marker, detail)
		}
	}

	// Aggregate: raw mean 2/5 = 0.4, nadir-normalized (ADR 0009) to
	// (0.4 - 0.25) / 0.75 = 0.2.
	if score, ok := run["score"].(float64); !ok || math.Abs(score-0.2) > 1e-9 {
		t.Errorf("run score = %v, want 0.2 (nadir-normalized 0.4 raw)", run["score"])
	}
}
