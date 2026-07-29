package server_test

import (
	"strings"
	"testing"
)

// Black-box coverage for the authoritative-benchmark pipeline's first suite
// (ticket 94, spec 0014 decision C, ADR 0013): the MMLU knowledge suite is
// seeded from the embedded frozen subset, the mcq rule verdict extracts and
// matches option letters with conservative normalization, and a campaign
// containing the suite reports a knowledge-dimension score. Zero LLM judge
// calls are involved anywhere in these scenarios.

// mmluCaseFields extracts the fields these tests need from one case entry of
// the suite listing.
type mmluCaseFields struct {
	id       int64
	prompt   string
	expected string
}

// mmluSuiteCases fetches the seeded mmlu suite and returns its cases with
// their mcq expectations, failing on any shape drift.
func mmluSuiteCases(t *testing.T, base string) (map[string]interface{}, []mmluCaseFields) {
	t.Helper()
	suite := suiteByKey(t, base, "mmlu")
	raw, ok := suite["cases"].([]interface{})
	if !ok {
		t.Fatalf("mmlu suite cases missing: %v", suite)
	}
	cases := make([]mmluCaseFields, 0, len(raw))
	for _, c := range raw {
		cm := c.(map[string]interface{})
		rc, ok := cm["rule_config"].(map[string]interface{})
		if !ok {
			t.Fatalf("mmlu case %v missing rule_config", cm["id"])
		}
		cases = append(cases, mmluCaseFields{
			id:       int64(cm["id"].(float64)),
			prompt:   cm["prompt"].(string),
			expected: rc["expected"].(string),
		})
	}
	return suite, cases
}

// correctVariants renders the correct option letter in the phrasings the mcq
// verdict must normalize to a hit.
var correctVariants = []func(letter string) string{
	func(l string) string { return l },                            // "B"
	func(l string) string { return strings.ToLower(l) },           // "b"
	func(l string) string { return "答案是 " + strings.ToLower(l) },  // "答案是 b"
	func(l string) string { return "The answer is " + l + "." },   // "The answer is B."
	func(l string) string { return "(" + l + ")" },                // "(B)"
	func(l string) string { return l + "." },                      // "B."
	func(l string) string { return "选" + l },                      // "选B"
	func(l string) string { return "The answer is (" + l + ")." }, // "The answer is (B)."
	func(l string) string { return "答案是 " + fullWidthLetter(l) },  // "答案是 Ｂ" (NFKC fold)
}

// fullWidthLetter maps A-D onto their full-width forms (U+FF21..U+FF24), so
// the NFKC folding in the mcq normalization is exercised end to end.
func fullWidthLetter(l string) string {
	if len(l) == 1 && l[0] >= 'A' && l[0] <= 'D' {
		return string(rune(0xFF21) + rune(l[0]-'A'))
	}
	return l
}

// wrongOptionLetter returns a different option letter than expected.
func wrongOptionLetter(expected string) string {
	for _, l := range []string{"A", "B", "C", "D"} {
		if l != expected {
			return l
		}
	}
	return "A"
}

// wrongVariants renders a wrong or unscoreable answer for case i: wrong
// letters in several phrasings, prose with no extractable letter, and
// conflicting letters (extraction must conservatively fail, scoring 0).
func wrongVariants(i int, expected string) string {
	wrong := wrongOptionLetter(expected)
	switch i % 5 {
	case 0:
		return wrong
	case 1:
		return "The answer is " + wrong + "."
	case 2:
		return "答案是" + wrong
	case 3:
		return "Hmm, this one is tricky and I am not really sure."
	default:
		return "答案是 A,正确答案是 B"
	}
}

// TestMMLUSuiteSeeded asserts the benchmark seed casts the mmlu suite from
// the embedded subset: capability knowledge, in the rotation since the
// ticket-99 cutover, the four-option nadir floor, and 100 single-sample mcq
// cases.
func TestMMLUSuiteSeeded(t *testing.T) {
	ts, _, _ := setupEvalEnv(t)

	suite, cases := mmluSuiteCases(t, ts.URL)
	if suite["capability"] != "knowledge" {
		t.Errorf("mmlu capability = %v, want knowledge", suite["capability"])
	}
	// Post-cutover (ticket 99): the benchmark suite is the rotation.
	if suite["enabled"] != true {
		t.Errorf("mmlu enabled = %v, want true (post-cutover rotation)", suite["enabled"])
	}
	if suite["nadir"] != 0.25 {
		t.Errorf("mmlu nadir = %v, want 0.25 (four-option floor)", suite["nadir"])
	}
	if len(cases) != 100 {
		t.Fatalf("mmlu cases = %d, want 100 (frozen subset)", len(cases))
	}

	raw := suite["cases"].([]interface{})
	for i, c := range raw {
		cm := c.(map[string]interface{})
		if cm["verdict_type"] != "rule" {
			t.Errorf("case %d verdict_type = %v, want rule (zero LLM judge)", i, cm["verdict_type"])
		}
		rc := cm["rule_config"].(map[string]interface{})
		if rc["mode"] != "mcq" {
			t.Errorf("case %d rule mode = %v, want mcq", i, rc["mode"])
		}
		if exp := rc["expected"].(string); !strings.Contains("A B C D", exp) || len(exp) != 1 {
			t.Errorf("case %d expected = %q, want one of A-D", i, exp)
		}
		if cm["sample_count"] != 1.0 {
			t.Errorf("case %d sample_count = %v, want 1", i, cm["sample_count"])
		}
		if cm["enabled"] != true {
			t.Errorf("case %d enabled = %v, want true", i, cm["enabled"])
		}
		prompt := cm["prompt"].(string)
		for _, marker := range []string{"A. ", "B. ", "C. ", "D. "} {
			if !strings.Contains(prompt, marker) {
				t.Errorf("case %d prompt missing option marker %q: %q", i, marker, prompt)
			}
		}
	}
}

// TestMMLUSeedIdempotent reopens the database and asserts the benchmark seed
// never re-casts cases and never reverts an admin edit (generation semantics
// of the seed bank, ticket 94 acceptance).
func TestMMLUSeedIdempotent(t *testing.T) {
	dbPath := t.TempDir() + "/mmlu.db"

	ts, db := openSuitesServer(t, dbPath)
	_, cases := mmluSuiteCases(t, ts.URL)
	if len(cases) != 100 {
		t.Fatalf("first boot mmlu cases = %d, want 100", len(cases))
	}
	// Admin curation: disable one case in place.
	patchCase(t, ts.URL, cases[0].id, map[string]interface{}{"enabled": false})
	db.Close()

	ts2, db2 := openSuitesServer(t, dbPath)
	defer db2.Close()
	suite, cases2 := mmluSuiteCases(t, ts2.URL)
	if len(cases2) != 100 {
		t.Errorf("second boot mmlu cases = %d, want 100 (no seed duplicates)", len(cases2))
	}
	if v := suite["version"]; v != 2.0 {
		t.Errorf("mmlu version after reopen = %v, want 2 (seed must not rebump)", v)
	}
	first := caseByID(t, suite, cases[0].id)
	if first["enabled"] != false {
		t.Errorf("admin disable reverted by reseed: %v", first)
	}

	db2.Close()
	ts3, db3 := openSuitesServer(t, dbPath)
	defer db3.Close()
	_, cases3 := mmluSuiteCases(t, ts3.URL)
	if len(cases3) != 100 {
		t.Errorf("third boot mmlu cases = %d, want still 100", len(cases3))
	}
}

// TestMMLUMCQRuleVerdicts runs the mmlu suite against a model that answers
// every question correctly in rotating phrasings and a model that answers
// wrong or unscoreably, asserting the mcq rule verdict scores 1/0
// accordingly — with zero judge involvement.
func TestMMLUMCQRuleVerdicts(t *testing.T) {
	ts, stubSmart, _ := setupEvalEnv(t)
	stubDumb := newEvalStubHub()
	t.Cleanup(stubDumb.Close)

	smartID := createEvalModel(t, ts.URL, stubSmart.URL, "mmlu-smart")
	dumbID := createEvalModel(t, ts.URL, stubDumb.URL, "mmlu-dumb")

	suite, cases := mmluSuiteCases(t, ts.URL)
	suiteID := int64(suite["id"].(float64))
	for i, c := range cases {
		stubSmart.setAnswerSeq(c.prompt, correctVariants[i%len(correctVariants)](c.expected))
		stubDumb.setAnswerSeq(c.prompt, wrongVariants(i, c.expected))
	}

	runID := triggerEval(t, ts.URL, suiteID, smartID, dumbID)
	run := waitEvalDone(t, ts.URL, runID)
	if run["status"] != "done" {
		t.Fatalf("run status = %v, want done", run["status"])
	}

	smart := resultsByModel(run, "mmlu-smart")
	if len(smart) != 100 {
		t.Fatalf("smart model has %d results, want 100", len(smart))
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
		if calls := stubSmart.callCount("mmlu-smart", c.prompt); calls != 1 {
			t.Errorf("case %d answered %d times, want 1 (sample_count=1)", c.id, calls)
		}
	}

	dumb := resultsByModel(run, "mmlu-dumb")
	if len(dumb) != 100 {
		t.Fatalf("dumb model has %d results, want 100", len(dumb))
	}
	for _, r := range dumb {
		if r["score"] != 0.0 {
			t.Errorf("dumb case %v score = %v, want 0 (answer %q, detail: %v)",
				r["case_id"], r["score"], r["answer_text"], r["verdict_detail"])
		}
		detail := r["verdict_detail"].(string)
		if !strings.Contains(detail, "rule mcq") || strings.Contains(detail, "matched") {
			t.Errorf("dumb verdict_detail should report the mcq miss: %v", detail)
		}
	}

	// Aggregate: raw mean (100 x 1 + 100 x 0) / 200 = 0.5, nadir-normalized
	// (ADR 0009) to (0.5 - 0.25) / 0.75 = 1/3.
	if score, ok := run["score"].(float64); !ok || score != 1.0/3.0 {
		t.Errorf("run score = %v, want 1/3 (nadir-normalized 0.5 raw)", run["score"])
	}
}

// TestMMLUCampaignReportShowsKnowledge runs a campaign containing the mmlu
// suite and asserts the report carries the knowledge-dimension score.
func TestMMLUCampaignReportShowsKnowledge(t *testing.T) {
	ts, stub, _ := setupEvalEnv(t)
	modelID := createEvalModel(t, ts.URL, stub.URL, "mmlu-ace")

	suite, cases := mmluSuiteCases(t, ts.URL)
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
		if s.(map[string]interface{})["key"] == "mmlu" {
			found = true
		}
	}
	if !found {
		t.Errorf("report suites should cover mmlu: %v", suites)
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
	if scores["mmlu"] != 100.0 {
		t.Errorf("knowledge dimension (mmlu) score = %v, want 100 (all correct, nadir-normalized)", scores["mmlu"])
	}
	if row["total_score"] != 100.0 {
		t.Errorf("total_score = %v, want 100", row["total_score"])
	}
}

// TestMMLUMCQCaseValidation covers the admin API boundary for mcq cases
// (ADR 0013): the expectation must be a single option letter A-D, anything
// else could never score a hit and is rejected at creation.
func TestMMLUMCQCaseValidation(t *testing.T) {
	ts, _, _ := setupEvalEnv(t)
	suiteID := suiteIDByKey(t, ts.URL, "mmlu")

	create := func(expected string) int {
		resp := doPost(t, ts.URL+"/api/cases", map[string]interface{}{
			"suite_id":     suiteID,
			"prompt":       "Pick the right option. A. x B. y C. z D. w",
			"verdict_type": "rule",
			"rule_config":  map[string]string{"mode": "mcq", "expected": expected},
		})
		defer resp.Body.Close()
		return resp.StatusCode
	}

	for _, bad := range []string{"", "E", "AB", "1", "答案是B"} {
		if code := create(bad); code != 400 {
			t.Errorf("mcq expected %q: got %d, want 400", bad, code)
		}
	}
	for _, good := range []string{"A", "b", " C "} {
		if code := create(good); code != 201 {
			t.Errorf("mcq expected %q: got %d, want 201", good, code)
		}
	}
}
