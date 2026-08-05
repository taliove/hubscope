package server_test

import (
	"strings"
	"testing"
)

// Black-box coverage for the Chinese-language authoritative-benchmark suite
// (ticket 96, spec 0014 decision C, ADR 0013): the agieval_zh suite is seeded
// from the embedded frozen subset of AGIEval's MIT-licensed Gaokao Chinese
// MCQ tasks, the mcq rule verdict (shared with the mmlu suite) normalizes
// Chinese answer phrasings — full-width letters, 「选B」「答案是B」「答案:B」 —
// and a campaign containing the suite reports a language-dimension score.
// Zero LLM judge calls are involved anywhere in these scenarios.

// agievalCaseFields extracts the fields these tests need from one case entry
// of the suite listing.
type agievalCaseFields struct {
	id       int64
	prompt   string
	expected string
}

// agievalSuiteCases fetches the seeded agieval_zh suite and returns its cases
// with their mcq expectations, failing on any shape drift.
func agievalSuiteCases(t *testing.T, base string) (map[string]interface{}, []agievalCaseFields) {
	t.Helper()
	suite := suiteByKey(t, base, "agieval_zh")
	raw, ok := suite["cases"].([]interface{})
	if !ok {
		t.Fatalf("agieval_zh suite cases missing: %v", suite)
	}
	cases := make([]agievalCaseFields, 0, len(raw))
	for _, c := range raw {
		cm := c.(map[string]interface{})
		rc, ok := cm["rule_config"].(map[string]interface{})
		if !ok {
			t.Fatalf("agieval_zh case %v missing rule_config", cm["id"])
		}
		cases = append(cases, agievalCaseFields{
			id:       int64(cm["id"].(float64)),
			prompt:   cm["prompt"].(string),
			expected: rc["expected"].(string),
		})
	}
	return suite, cases
}

// zhCorrectVariants extends the shared mcq correct phrasings with the Chinese
// idioms the gaokao suite must score as hits: full-width colon, 「选 B」 with a
// space, 「正确答案是…」, a bare full-width letter, and a full-width trailing
// period.
var zhCorrectVariants = append(append([]func(string) string{}, correctVariants...),
	func(l string) string { return "答案：" + l },          // "答案:B" (full-width colon)
	func(l string) string { return "选 " + l },           // "选 B"
	func(l string) string { return "正确答案是" + l },        // "正确答案是B"
	func(l string) string { return fullWidthLetter(l) }, // "Ｂ" (bare full-width)
	func(l string) string { return l + "。" },            // "B。" (full-width period)
)

// TestAgievalZhSuiteSeeded asserts the benchmark seed casts the agieval_zh
// suite from the embedded subset: capability language, in the rotation since
// the ticket-99 cutover, the four-option nadir floor, and 100 single-sample
// mcq cases with the frozen Chinese prompt template.
func TestAgievalZhSuiteSeeded(t *testing.T) {
	ts, _, _ := setupEvalEnv(t)

	suite, cases := agievalSuiteCases(t, ts.URL)
	if suite["capability"] != "language" {
		t.Errorf("agieval_zh capability = %v, want language", suite["capability"])
	}
	// Post-cutover (ticket 99): the benchmark suite is the rotation.
	if suite["enabled"] != true {
		t.Errorf("agieval_zh enabled = %v, want true (post-cutover rotation)", suite["enabled"])
	}
	if suite["nadir"] != 0.25 {
		t.Errorf("agieval_zh nadir = %v, want 0.25 (four-option floor)", suite["nadir"])
	}
	if len(cases) != 20 {
		t.Fatalf("agieval_zh cases = %d, want 20 (frozen subset)", len(cases))
	}

	for i, c := range cases {
		if !strings.HasPrefix(c.prompt, "以下是一道单项选择题") {
			t.Errorf("case %d prompt should open with the frozen Chinese instruction: %q", i, c.prompt[:min(60, len(c.prompt))])
		}
		for _, marker := range []string{"A. ", "B. ", "C. ", "D. "} {
			if !strings.Contains(c.prompt, marker) {
				t.Errorf("case %d prompt missing option marker %q", i, marker)
			}
		}
		if !strings.Contains("A B C D", c.expected) || len(c.expected) != 1 {
			t.Errorf("case %d expected = %q, want one of A-D", i, c.expected)
		}
	}

	// Passage composition: the first gaokao-chinese row carries its reading
	// material; the case cast from it must embed the passage before the
	// question (frozen prompt, W7).
	found := false
	for _, c := range cases {
		if strings.Contains(c.prompt, "下列对材料相关内容的理解和分析") {
			found = true
			if !strings.Contains(c.prompt, "历史地理学的起源") {
				t.Errorf("gaokao-chinese reading-comprehension case lost its passage: %q", c.prompt[:min(120, len(c.prompt))])
			}
		}
	}
	if !found {
		t.Errorf("no case carries the first gaokao-chinese question; subset drifted?")
	}
}

// TestAgievalZhSeedIdempotent reopens the database and asserts the benchmark
// seed never re-casts cases and never reverts an admin edit (generation
// semantics, same contract as the mmlu suite).
func TestAgievalZhSeedIdempotent(t *testing.T) {
	dbPath := t.TempDir() + "/agieval_zh.db"

	ts, db := openSuitesServer(t, dbPath)
	_, cases := agievalSuiteCases(t, ts.URL)
	if len(cases) != 20 {
		t.Fatalf("first boot agieval_zh cases = %d, want 20", len(cases))
	}
	// Admin curation: disable one case in place.
	patchCase(t, ts.URL, cases[0].id, map[string]interface{}{"enabled": false})
	db.Close()

	ts2, db2 := openSuitesServer(t, dbPath)
	defer db2.Close()
	suite, cases2 := agievalSuiteCases(t, ts2.URL)
	if len(cases2) != 20 {
		t.Errorf("second boot agieval_zh cases = %d, want 20 (no seed duplicates)", len(cases2))
	}
	if v := suite["version"]; v != 2.0 {
		t.Errorf("agieval_zh version after reopen = %v, want 2 (seed must not rebump)", v)
	}
	first := caseByID(t, suite, cases[0].id)
	if first["enabled"] != false {
		t.Errorf("admin disable reverted by reseed: %v", first)
	}

	db2.Close()
	ts3, db3 := openSuitesServer(t, dbPath)
	defer db3.Close()
	_, cases3 := agievalSuiteCases(t, ts3.URL)
	if len(cases3) != 20 {
		t.Errorf("third boot agieval_zh cases = %d, want still 20", len(cases3))
	}
}

// TestAgievalZhMCQRuleVerdicts runs the agieval_zh suite against a model that
// answers every question correctly in rotating Chinese phrasings and a model
// that answers wrong or unscoreably, asserting the mcq rule verdict scores
// 1/0 accordingly — with zero judge involvement.
func TestAgievalZhMCQRuleVerdicts(t *testing.T) {
	ts, stubSmart, _ := setupEvalEnv(t)
	stubDumb := newEvalStubHub()
	t.Cleanup(stubDumb.Close)

	smartID := createEvalModel(t, ts.URL, stubSmart.URL, "agieval-smart")
	dumbID := createEvalModel(t, ts.URL, stubDumb.URL, "agieval-dumb")

	suite, cases := agievalSuiteCases(t, ts.URL)
	suiteID := int64(suite["id"].(float64))
	for i, c := range cases {
		stubSmart.setAnswerSeq(c.prompt, zhCorrectVariants[i%len(zhCorrectVariants)](c.expected))
		stubDumb.setAnswerSeq(c.prompt, wrongVariants(i, c.expected))
	}

	runID := triggerEval(t, ts.URL, suiteID, smartID, dumbID)
	run := waitEvalDone(t, ts.URL, runID)
	if run["status"] != "done" {
		t.Fatalf("run status = %v, want done", run["status"])
	}

	smart := resultsByModel(run, "agieval-smart")
	if len(smart) != 20 {
		t.Fatalf("smart model has %d results, want 20", len(smart))
	}
	for _, r := range smart {
		if r["score"] != 1.0 {
			t.Errorf("smart case %v score = %v, want 1 (answer %q, detail: %v)",
				r["case_id"], r["score"], r["answer_text"], r["verdict_detail"])
		}
		detail := r["verdict_detail"].(string)
		if !strings.Contains(detail, "rule mcq matched") {
			t.Errorf("smart verdict_detail should explain the mcq match: %v", detail)
		}
		if strings.Contains(detail, "judge") {
			t.Errorf("rule verdict must never involve the judge: %v", detail)
		}
	}
	// sample_count = 1: exactly one completion call per case.
	for _, c := range cases[:5] {
		if calls := stubSmart.callCount("agieval-smart", c.prompt); calls != 1 {
			t.Errorf("case %d answered %d times, want 1 (sample_count=1)", c.id, calls)
		}
	}

	dumb := resultsByModel(run, "agieval-dumb")
	if len(dumb) != 20 {
		t.Fatalf("dumb model has %d results, want 20", len(dumb))
	}
	for _, r := range dumb {
		if r["score"] != 0.0 {
			t.Errorf("dumb case %v score = %v, want 0 (answer %q, detail: %v)",
				r["case_id"], r["score"], r["answer_text"], r["verdict_detail"])
		}
	}

	// Aggregate: raw mean (100 x 1 + 100 x 0) / 200 = 0.5, nadir-normalized
	// (ADR 0009) to (0.5 - 0.25) / 0.75 = 1/3.
	if score, ok := run["score"].(float64); !ok || score != 1.0/3.0 {
		t.Errorf("run score = %v, want 1/3 (nadir-normalized 0.5 raw)", run["score"])
	}
}

// TestAgievalZhCampaignReportShowsLanguage runs a campaign containing the
// agieval_zh suite and asserts the report carries the language-dimension
// score.
func TestAgievalZhCampaignReportShowsLanguage(t *testing.T) {
	ts, stub, _ := setupEvalEnv(t)
	modelID := createEvalModel(t, ts.URL, stub.URL, "agieval-ace")

	suite, cases := agievalSuiteCases(t, ts.URL)
	suiteID := int64(suite["id"].(float64))
	for _, c := range cases {
		stub.setAnswerSeq(c.prompt, c.expected)
	}

	runID := triggerEval(t, ts.URL, suiteID, modelID)
	run := waitEvalDone(t, ts.URL, runID)
	if run["status"] != "done" {
		t.Fatalf("run status = %v, want done", run["status"])
	}
	campaignID := int64(run["campaign_id"].(float64))
	final := waitCampaignStatus(t, ts.URL, campaignID, "done", "failed")
	if final["status"] != "done" {
		t.Fatalf("campaign status = %v, want done", final["status"])
	}

	report := getCampaignReport(t, ts.URL, campaignID, "")
	suites, ok := report["suites"].([]interface{})
	if !ok {
		t.Fatalf("report suites missing: %v", report)
	}
	found := false
	for _, s := range suites {
		if s.(map[string]interface{})["key"] == "agieval_zh" {
			found = true
		}
	}
	if !found {
		t.Errorf("report suites should cover agieval_zh: %v", suites)
	}

	rows, ok := report["rows"].([]interface{})
	if !ok || len(rows) != 1 {
		t.Fatalf("report rows = %v, want exactly 1", rows)
	}
	row := rows[0].(map[string]interface{})
	scores, ok := row["suite_scores"].(map[string]interface{})
	if !ok {
		t.Fatalf("row suite_scores missing: %v", row)
	}
	if scores["agieval_zh"] != 100.0 {
		t.Errorf("language dimension (agieval_zh) score = %v, want 20 (all correct, nadir-normalized)", scores["agieval_zh"])
	}
	if row["total_score"] != 100.0 {
		t.Errorf("total_score = %v, want 20", row["total_score"])
	}
}
