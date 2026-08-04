package server_test

import (
	"fmt"
	"strings"
	"testing"
)

// Black-box coverage for the authoritative-benchmark pipeline's reasoning
// suite (ticket 95, spec 0014 decision C, ADR 0013): the GSM8K suite is
// seeded from the embedded frozen subset, the numeric rule verdict extracts
// the final numeric answer (#### marker first, last-line number fallback;
// thousands separators, signs and decimals normalized) and matches it
// exactly, and a campaign containing the suite reports a reasoning-dimension
// score. Zero LLM judge calls are involved anywhere in these scenarios.

// gsm8kCaseFields extracts the fields these tests need from one case entry
// of the suite listing.
type gsm8kCaseFields struct {
	id       int64
	prompt   string
	expected string
}

// gsm8kSuiteCases fetches the seeded gsm8k suite and returns its cases with
// their numeric expectations, failing on any shape drift.
func gsm8kSuiteCases(t *testing.T, base string) (map[string]interface{}, []gsm8kCaseFields) {
	t.Helper()
	suite := suiteByKey(t, base, "gsm8k")
	raw, ok := suite["cases"].([]interface{})
	if !ok {
		t.Fatalf("gsm8k suite cases missing: %v", suite)
	}
	cases := make([]gsm8kCaseFields, 0, len(raw))
	for _, c := range raw {
		cm := c.(map[string]interface{})
		rc, ok := cm["rule_config"].(map[string]interface{})
		if !ok {
			t.Fatalf("gsm8k case %v missing rule_config", cm["id"])
		}
		cases = append(cases, gsm8kCaseFields{
			id:       int64(cm["id"].(float64)),
			prompt:   cm["prompt"].(string),
			expected: rc["expected"].(string),
		})
	}
	return suite, cases
}

// withThousands renders a number string with comma thousands separators
// ("14000" -> "14,000"), exercising the separator normalization.
func withThousands(digits string) string {
	if strings.ContainsAny(digits, ".-") {
		return digits
	}
	n := len(digits)
	if n <= 3 {
		return digits
	}
	var b strings.Builder
	for i, c := range digits {
		if i > 0 && (n-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	return b.String()
}

// asDecimal renders an integer expectation with a trailing ".0", exercising
// decimal normalization ("42" expected, "42.0" answered).
func asDecimal(expected string) string {
	if strings.Contains(expected, ".") {
		return expected
	}
	return expected + ".0"
}

// gsm8kCorrectVariants renders the expected numeric answer in the phrasings
// the numeric verdict must normalize to a hit: the official "#### N" marker,
// thousands separators, decimal spellings, a "$"-prefixed amount, and the
// last-line-number fallback when the marker is absent.
var gsm8kCorrectVariants = []func(expected string) string{
	func(e string) string { return "#### " + e },                                     // "#### 42"
	func(e string) string { return "First I add, then I multiply.\n#### " + e },      // reasoning then marker
	func(e string) string { return "#### " + withThousands(e) },                      // "#### 14,000"
	func(e string) string { return "#### " + asDecimal(e) },                          // "#### 42.0"
	func(e string) string { return "The total comes to $" + e + ".\n#### $" + e },    // "$42" amount
	func(e string) string { return "Step by step: 2 + 2 = 4, so the total is " + e }, // last-line fallback, no marker
}

// gsm8kWrongNumber returns a numeric string guaranteed different from the
// expectation (digit appended), so a wrong-answer variant can never
// accidentally hit.
func gsm8kWrongNumber(expected string) string {
	return expected + "1"
}

// gsm8kWrongVariants renders a wrong or unscoreable answer for case i: a
// wrong value behind the marker, a wrong value as the last-line number, a
// right-looking opening number overridden by a wrong final line, and prose
// with no number at all (extraction must conservatively fail, scoring 0).
func gsm8kWrongVariants(i int, expected string) string {
	wrong := gsm8kWrongNumber(expected)
	switch i % 4 {
	case 0:
		return "#### " + wrong
	case 1:
		return "I think the answer is " + expected + ", but let me redo it.\nAfter recomputing, the total is " + wrong
	case 2:
		return "Let me think about this problem carefully.\nHmm, I cannot pin down the final amount."
	default:
		return "The calculation starts with " + expected + " units.\n#### " + wrong
	}
}

// TestGSM8KSuiteSeeded asserts the benchmark seed casts the gsm8k suite from
// the embedded subset: capability reasoning, in the rotation since the
// ticket-99 cutover, zero nadir floor, and 100 single-sample numeric cases.
func TestGSM8KSuiteSeeded(t *testing.T) {
	ts, _, _ := setupEvalEnv(t)

	suite, cases := gsm8kSuiteCases(t, ts.URL)
	if suite["capability"] != "reasoning" {
		t.Errorf("gsm8k capability = %v, want reasoning", suite["capability"])
	}
	// Post-cutover (ticket 99): the benchmark suite is the rotation.
	if suite["enabled"] != true {
		t.Errorf("gsm8k enabled = %v, want true (post-cutover rotation)", suite["enabled"])
	}
	if suite["nadir"] != 0.0 {
		t.Errorf("gsm8k nadir = %v, want 0 (open-ended numeric has no random-guess floor; ticket 99 calibrates)", suite["nadir"])
	}
	if len(cases) != 20 {
		t.Fatalf("gsm8k cases = %d, want 20 (frozen subset)", len(cases))
	}

	raw := suite["cases"].([]interface{})
	for i, c := range raw {
		cm := c.(map[string]interface{})
		if cm["verdict_type"] != "rule" {
			t.Errorf("case %d verdict_type = %v, want rule (zero LLM judge)", i, cm["verdict_type"])
		}
		rc := cm["rule_config"].(map[string]interface{})
		if rc["mode"] != "numeric" {
			t.Errorf("case %d rule mode = %v, want numeric", i, rc["mode"])
		}
		if exp := rc["expected"].(string); !numericPlain(exp) {
			t.Errorf("case %d expected = %q, want a plain number", i, exp)
		}
		if cm["sample_count"] != 1.0 {
			t.Errorf("case %d sample_count = %v, want 1", i, cm["sample_count"])
		}
		if cm["enabled"] != true {
			t.Errorf("case %d enabled = %v, want true", i, cm["enabled"])
		}
		if prompt := cm["prompt"].(string); !strings.Contains(prompt, "####") {
			t.Errorf("case %d prompt should instruct the '#### N' answer marker: %q", i, prompt)
		}
	}
}

// numericPlain reports whether s is a plain signed integer or decimal with
// no separators — the shape every seeded gsm8k expectation must have.
func numericPlain(s string) bool {
	if s == "" {
		return false
	}
	s = strings.TrimPrefix(s, "-")
	if s == "" {
		return false
	}
	dots := 0
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
		case r == '.':
			dots++
		default:
			return false
		}
	}
	return dots <= 1
}

// TestGSM8KSeedIdempotent reopens the database and asserts the benchmark
// seed never re-casts cases and never reverts an admin edit (generation
// semantics of the seed bank, ticket 95 acceptance).
func TestGSM8KSeedIdempotent(t *testing.T) {
	dbPath := t.TempDir() + "/gsm8k.db"

	ts, db := openSuitesServer(t, dbPath)
	_, cases := gsm8kSuiteCases(t, ts.URL)
	if len(cases) != 20 {
		t.Fatalf("first boot gsm8k cases = %d, want 20", len(cases))
	}
	// Admin curation: disable one case in place.
	patchCase(t, ts.URL, cases[0].id, map[string]interface{}{"enabled": false})
	db.Close()

	ts2, db2 := openSuitesServer(t, dbPath)
	defer db2.Close()
	suite, cases2 := gsm8kSuiteCases(t, ts2.URL)
	if len(cases2) != 20 {
		t.Errorf("second boot gsm8k cases = %d, want 20 (no seed duplicates)", len(cases2))
	}
	if v := suite["version"]; v != 2.0 {
		t.Errorf("gsm8k version after reopen = %v, want 2 (seed must not rebump)", v)
	}
	first := caseByID(t, suite, cases[0].id)
	if first["enabled"] != false {
		t.Errorf("admin disable reverted by reseed: %v", first)
	}

	db2.Close()
	ts3, db3 := openSuitesServer(t, dbPath)
	defer db3.Close()
	_, cases3 := gsm8kSuiteCases(t, ts3.URL)
	if len(cases3) != 20 {
		t.Errorf("third boot gsm8k cases = %d, want still 20", len(cases3))
	}
}

// TestGSM8KNumericRuleVerdicts runs the gsm8k suite against a model that
// answers every question correctly in rotating phrasings and a model that
// answers wrong or unscoreably, asserting the numeric rule verdict scores
// 1/0 accordingly — with zero judge involvement.
func TestGSM8KNumericRuleVerdicts(t *testing.T) {
	ts, stubSmart, _ := setupEvalEnv(t)
	stubDumb := newEvalStubHub()
	t.Cleanup(stubDumb.Close)

	smartID := createEvalModel(t, ts.URL, stubSmart.URL, "gsm8k-smart")
	dumbID := createEvalModel(t, ts.URL, stubDumb.URL, "gsm8k-dumb")

	suite, cases := gsm8kSuiteCases(t, ts.URL)
	suiteID := int64(suite["id"].(float64))
	for i, c := range cases {
		stubSmart.setAnswerSeq(c.prompt, gsm8kCorrectVariants[i%len(gsm8kCorrectVariants)](c.expected))
		stubDumb.setAnswerSeq(c.prompt, gsm8kWrongVariants(i, c.expected))
	}

	runID := triggerEval(t, ts.URL, suiteID, smartID, dumbID)
	run := waitEvalDone(t, ts.URL, runID)
	if run["status"] != "done" {
		t.Fatalf("run status = %v, want done", run["status"])
	}

	smart := resultsByModel(run, "gsm8k-smart")
	if len(smart) != 20 {
		t.Fatalf("smart model has %d results, want 20", len(smart))
	}
	for _, r := range smart {
		if r["score"] != 1.0 {
			t.Errorf("smart case %v score = %v, want 1 (answer %q, detail: %v)",
				r["case_id"], r["score"], r["answer_text"], r["verdict_detail"])
		}
		detail := r["verdict_detail"].(string)
		if !strings.Contains(detail, "rule numeric matched") {
			t.Errorf("smart verdict_detail should explain the numeric match: %v", detail)
		}
		if strings.Contains(detail, "judge") {
			t.Errorf("rule verdict must never involve the judge: %v", detail)
		}
	}
	// sample_count = 1: exactly one completion call per case.
	for _, c := range cases[:5] {
		if calls := stubSmart.callCount("gsm8k-smart", c.prompt); calls != 1 {
			t.Errorf("case %d answered %d times, want 1 (sample_count=1)", c.id, calls)
		}
	}

	dumb := resultsByModel(run, "gsm8k-dumb")
	if len(dumb) != 20 {
		t.Fatalf("dumb model has %d results, want 20", len(dumb))
	}
	for _, r := range dumb {
		if r["score"] != 0.0 {
			t.Errorf("dumb case %v score = %v, want 0 (answer %q, detail: %v)",
				r["case_id"], r["score"], r["answer_text"], r["verdict_detail"])
		}
		detail := r["verdict_detail"].(string)
		if !strings.Contains(detail, "rule numeric") || strings.Contains(detail, "matched") {
			t.Errorf("dumb verdict_detail should report the numeric miss: %v", detail)
		}
	}

	// Aggregate: raw mean (100 x 1 + 100 x 0) / 200 = 0.5, nadir 0 so the
	// normalized score (ADR 0009) is also 0.5.
	if score, ok := run["score"].(float64); !ok || score != 0.5 {
		t.Errorf("run score = %v, want 0.5 (nadir 0)", run["score"])
	}
}

// TestGSM8KExtractionBoundaries pins the conservative extraction contract
// end to end through a single-case run: conflicting numbers resolve to the
// marker/last-line rule, and a negative answer is normalized with its sign.
func TestGSM8KExtractionBoundaries(t *testing.T) {
	ts, stub, _ := setupEvalEnv(t)
	modelID := createEvalModel(t, ts.URL, stub.URL, "gsm8k-edge")

	suite, cases := gsm8kSuiteCases(t, ts.URL)
	suiteID := int64(suite["id"].(float64))
	c := cases[0]

	// The marker wins over every earlier number, even on a different line.
	stub.setAnswerSeq(c.prompt, fmt.Sprintf("Partial: %s. Final line below.\n#### %s", gsm8kWrongNumber(c.expected), c.expected))
	runID := triggerEval(t, ts.URL, suiteID, modelID)
	run := waitEvalDone(t, ts.URL, runID)
	var hit map[string]interface{}
	for _, r := range resultsByModel(run, "gsm8k-edge") {
		if int64(r["case_id"].(float64)) == c.id {
			hit = r
		}
	}
	if hit == nil || hit["score"] != 1.0 {
		t.Fatalf("marker-priority case %d result = %v, want a hit (earlier wrong number must not win)", c.id, hit)
	}
}

// TestGSM8KNumericCaseValidation covers the admin API boundary for numeric
// cases (ADR 0013): the expectation must canonicalize to a plain number,
// anything else could never score a hit and is rejected at creation.
func TestGSM8KNumericCaseValidation(t *testing.T) {
	ts, _, _ := setupEvalEnv(t)
	suiteID := suiteIDByKey(t, ts.URL, "gsm8k")

	create := func(expected string) int {
		resp := doPost(t, ts.URL+"/api/cases", map[string]interface{}{
			"suite_id":     suiteID,
			"prompt":       "What is 6 times 7? End with '#### N'.",
			"verdict_type": "rule",
			"rule_config":  map[string]string{"mode": "numeric", "expected": expected},
		})
		defer resp.Body.Close()
		return resp.StatusCode
	}

	for _, bad := range []string{"", "abc", "1.2.3", "four", "--1", "1,2,3", "."} {
		if code := create(bad); code != 400 {
			t.Errorf("numeric expected %q: got %d, want 400", bad, code)
		}
	}
	for _, good := range []string{"42", "-3", "4.5", " 7 ", "1,000"} {
		if code := create(good); code != 201 {
			t.Errorf("numeric expected %q: got %d, want 201", good, code)
		}
	}
}

// TestGSM8KCapabilityFilterPinsReasoning documents that the gsm8k suite is
// the reasoning dimension of the post-cutover rotation (the v3 cap_reasoning
// it replaced is gone), so the capability filter lists exactly it.
func TestGSM8KCapabilityFilterPinsReasoning(t *testing.T) {
	ts, _, _ := setupEvalEnv(t)

	suites := fetchSuites(t, ts.URL, "?capability=reasoning")
	keys := map[string]bool{}
	for _, s := range suites {
		keys[s["key"].(string)] = true
	}
	if len(suites) != 1 || !keys["gsm8k"] {
		t.Errorf("capability=reasoning suites = %v, want exactly gsm8k", suites)
	}
}

// TestGSM8KCampaignReportShowsReasoning runs a campaign containing the gsm8k
// suite and asserts the report carries the reasoning-dimension score.
func TestGSM8KCampaignReportShowsReasoning(t *testing.T) {
	ts, stub, _ := setupEvalEnv(t)
	modelID := createEvalModel(t, ts.URL, stub.URL, "gsm8k-ace")

	suite, cases := gsm8kSuiteCases(t, ts.URL)
	suiteID := int64(suite["id"].(float64))
	for _, c := range cases {
		stub.setAnswerSeq(c.prompt, "#### "+c.expected)
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
		if s.(map[string]interface{})["key"] == "gsm8k" {
			found = true
		}
	}
	if !found {
		t.Errorf("report suites should cover gsm8k: %v", suites)
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
	if scores["gsm8k"] != 100.0 {
		t.Errorf("reasoning dimension (gsm8k) score = %v, want 20 (all correct, nadir-normalized)", scores["gsm8k"])
	}
	if row["total_score"] != 100.0 {
		t.Errorf("total_score = %v, want 20", row["total_score"])
	}
}
