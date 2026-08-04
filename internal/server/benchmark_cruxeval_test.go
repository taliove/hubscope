package server_test

import (
	"strings"
	"testing"
)

// Black-box coverage for the CRUXEval coding suite (ticket 98, spec 0014
// decision C, ADR 0013): the suite is seeded from the embedded frozen subset
// whose standard answers were precomputed offline (scripts/cruxeval_subset.py,
// dev-machine tool, not shipped), and the output_match rule verdict compares
// the model's predicted output literal against the standard answer with
// conservative normalization — whitespace/newlines, quote style, container
// spacing, and True/False/None casing normalize to a hit; wrong values and
// non-literal output score 0, never a guessed hit. The runtime never executes
// any code: scoring is pure literal comparison, zero sandbox, zero Python,
// zero LLM judge.

// cruxevalCaseFields extracts the fields these tests need from one case entry
// of the suite listing.
type cruxevalCaseFields struct {
	id       int64
	prompt   string
	expected string
}

// cruxevalSuiteCases fetches the seeded cruxeval suite and returns its cases
// with their output_match expectations, failing on any shape drift.
func cruxevalSuiteCases(t *testing.T, base string) (map[string]interface{}, []cruxevalCaseFields) {
	t.Helper()
	suite := suiteByKey(t, base, "cruxeval")
	raw, ok := suite["cases"].([]interface{})
	if !ok {
		t.Fatalf("cruxeval suite cases missing: %v", suite)
	}
	cases := make([]cruxevalCaseFields, 0, len(raw))
	for _, c := range raw {
		cm := c.(map[string]interface{})
		rc, ok := cm["rule_config"].(map[string]interface{})
		if !ok {
			t.Fatalf("cruxeval case %v missing rule_config", cm["id"])
		}
		cases = append(cases, cruxevalCaseFields{
			id:       int64(cm["id"].(float64)),
			prompt:   cm["prompt"].(string),
			expected: rc["expected"].(string),
		})
	}
	return suite, cases
}

// isBareLiteral reports whether the expected literal contains no string
// quotes, so whitespace/casing transforms cannot corrupt string contents.
func isBareLiteral(expected string) bool {
	return !strings.ContainsAny(expected, `'"`)
}

// swapQuotes renders a simple single-quoted string literal with double
// quotes; ok is false when the literal is not a safely swappable string.
func swapQuotes(expected string) (string, bool) {
	if len(expected) < 2 || expected[0] != '\'' || expected[len(expected)-1] != '\'' {
		return "", false
	}
	inner := expected[1 : len(expected)-1]
	if strings.ContainsAny(inner, `"\`) {
		return "", false
	}
	return `"` + inner + `"`, true
}

// outputCorrectVariants renders the standard answer in the shapes the
// output_match verdict must normalize to a hit. Each variant falls back to
// the verbatim literal when its transform does not apply to this expected.
var outputCorrectVariants = []func(expected string) string{
	func(e string) string { return e },                     // verbatim literal
	func(e string) string { return " \n\t " + e + "\n\n" }, // surrounding whitespace/newlines
	func(e string) string { // quote style: 'a' -> "a"
		if swapped, ok := swapQuotes(e); ok {
			return swapped
		}
		return e
	},
	func(e string) string { // container spacing: [1, 2] -> [1,2] (bare literals only)
		if isBareLiteral(e) {
			return strings.ReplaceAll(e, ", ", ",")
		}
		return e
	},
	func(e string) string { // container spacing: [1,2] -> [ 1 , 2 ] (bare literals only)
		if isBareLiteral(e) {
			e = strings.ReplaceAll(e, ",", " , ")
			e = strings.ReplaceAll(e, "[", "[ ")
			e = strings.ReplaceAll(e, "]", " ]")
			e = strings.ReplaceAll(e, "{", "{ ")
			e = strings.ReplaceAll(e, "}", " }")
			e = strings.ReplaceAll(e, "(", "( ")
			e = strings.ReplaceAll(e, ")", " )")
			e = strings.ReplaceAll(e, ":", " : ")
			return e
		}
		return e
	},
	func(e string) string { // True/False/None casing (bare literals only)
		if isBareLiteral(e) {
			e = strings.ReplaceAll(e, "True", "true")
			e = strings.ReplaceAll(e, "False", "false")
			e = strings.ReplaceAll(e, "None", "none")
			return e
		}
		return e
	},
	func(e string) string { return "```python\n" + e + "\n```" },      // code fence
	func(e string) string { return "assert f(x) == " + e },            // full assertion
	func(e string) string { return "[ANSWER]\n" + e + "\n[/ANSWER]" }, // official answer tags
	func(e string) string { return "( " + e + " )" },                  // bare grouping is the value itself
}

// outputWrongVariants renders a wrong or unscoreable answer for case i:
// mutated literals, plain prose, unsimplified expressions, and trailing
// garbage — every one must score 0 (extraction/parse failure is a miss,
// never a guess).
func outputWrongVariants(i int, expected string) string {
	switch i % 6 {
	case 0:
		return mutateLiteral(expected)
	case 1:
		return "Let me trace through the code. The function builds a result step by step."
	case 2:
		return "1 + 1" // unsimplified expression, not a literal
	case 3:
		return expected + " " + expected // two literals, ambiguous
	case 4:
		return "(" + expected + ",)" // a 1-tuple is never the bare value
	default:
		return "The output is " + mutateLiteral(expected)
	}
}

// mutateLiteral returns a literal of the same shape with a different value.
func mutateLiteral(expected string) string {
	switch {
	case expected == "True":
		return "False"
	case expected == "False":
		return "True"
	case expected == "None":
		return "0"
	case expected == "[]":
		return "[0]"
	case expected == "{}":
		return `{"__x__": 0}`
	case expected == "()":
		return "(0,)"
	case len(expected) >= 2 && expected[0] == '\'' && expected[len(expected)-1] == '\'':
		return expected[:len(expected)-1] + "Zzz" + "'"
	case len(expected) >= 2 && expected[0] == '"' && expected[len(expected)-1] == '"':
		return expected[:len(expected)-1] + "Zzz" + `"`
	case len(expected) >= 1 && (expected[0] == '[' || expected[0] == '(' || expected[0] == '{'):
		if expected[0] == '{' {
			return "{0: 0, " + expected[1:]
		}
		return string(expected[0]) + "0, " + expected[1:]
	default:
		// Numbers (and anything unrecognized): append a digit.
		return expected + "7"
	}
}

// TestCRUXEvalSuiteSeeded asserts the benchmark seed casts the cruxeval suite
// from the embedded subset: capability coding, in the rotation since the
// ticket-99 cutover, nadir 0
// pending ticket-99 calibration, and 100 single-sample output_match cases
// whose prompts carry the official-style output-prediction template.
func TestCRUXEvalSuiteSeeded(t *testing.T) {
	ts, _, _ := setupEvalEnv(t)

	suite, cases := cruxevalSuiteCases(t, ts.URL)
	if suite["capability"] != "coding" {
		t.Errorf("cruxeval capability = %v, want coding", suite["capability"])
	}
	// Post-cutover (ticket 99): the benchmark suite is the rotation.
	if suite["enabled"] != true {
		t.Errorf("cruxeval enabled = %v, want true (post-cutover rotation)", suite["enabled"])
	}
	if suite["nadir"] != 0.0 {
		t.Errorf("cruxeval nadir = %v, want 0 (output prediction has no guess floor; ticket 99 calibrates)", suite["nadir"])
	}
	if len(cases) != 20 {
		t.Fatalf("cruxeval cases = %d, want 20 (frozen subset)", len(cases))
	}

	raw := suite["cases"].([]interface{})
	for i, c := range raw {
		cm := c.(map[string]interface{})
		if cm["verdict_type"] != "rule" {
			t.Errorf("case %d verdict_type = %v, want rule (zero LLM judge)", i, cm["verdict_type"])
		}
		rc := cm["rule_config"].(map[string]interface{})
		if rc["mode"] != "output_match" {
			t.Errorf("case %d rule mode = %v, want output_match", i, rc["mode"])
		}
		if rc["expected"].(string) == "" {
			t.Errorf("case %d expected is empty, want the precomputed standard output", i)
		}
		if cm["sample_count"] != 1.0 {
			t.Errorf("case %d sample_count = %v, want 1", i, cm["sample_count"])
		}
		if cm["enabled"] != true {
			t.Errorf("case %d enabled = %v, want true", i, cm["enabled"])
		}
		prompt := cm["prompt"].(string)
		if !strings.Contains(prompt, "assert f(") || !strings.Contains(prompt, "def f(") {
			t.Errorf("case %d prompt missing the function or assertion: %q", i, prompt)
		}
	}
}

// TestCRUXEvalSeedIdempotent reopens the database and asserts the benchmark
// seed never re-casts cases and never reverts an admin edit (generation
// semantics of the seed bank, ticket 98 acceptance).
func TestCRUXEvalSeedIdempotent(t *testing.T) {
	dbPath := t.TempDir() + "/cruxeval.db"

	ts, db := openSuitesServer(t, dbPath)
	_, cases := cruxevalSuiteCases(t, ts.URL)
	if len(cases) != 20 {
		t.Fatalf("first boot cruxeval cases = %d, want 20", len(cases))
	}
	// Admin curation: disable one case in place.
	patchCase(t, ts.URL, cases[0].id, map[string]interface{}{"enabled": false})
	db.Close()

	ts2, db2 := openSuitesServer(t, dbPath)
	defer db2.Close()
	suite, cases2 := cruxevalSuiteCases(t, ts2.URL)
	if len(cases2) != 20 {
		t.Errorf("second boot cruxeval cases = %d, want 20 (no seed duplicates)", len(cases2))
	}
	if v := suite["version"]; v != 2.0 {
		t.Errorf("cruxeval version after reopen = %v, want 2 (seed must not rebump)", v)
	}
	first := caseByID(t, suite, cases[0].id)
	if first["enabled"] != false {
		t.Errorf("admin disable reverted by reseed: %v", first)
	}

	db2.Close()
	ts3, db3 := openSuitesServer(t, dbPath)
	defer db3.Close()
	_, cases3 := cruxevalSuiteCases(t, ts3.URL)
	if len(cases3) != 20 {
		t.Errorf("third boot cruxeval cases = %d, want still 20", len(cases3))
	}
}

// TestCRUXEvalOutputMatchVerdicts runs the cruxeval suite against a model
// that predicts every output correctly in rotating literal shapes and a model
// that answers wrong or unscoreably, asserting the output_match rule verdict
// scores 1/0 accordingly — pure literal comparison, zero execution, zero
// judge involvement.
func TestCRUXEvalOutputMatchVerdicts(t *testing.T) {
	ts, stubSmart, _ := setupEvalEnv(t)
	stubDumb := newEvalStubHub()
	t.Cleanup(stubDumb.Close)

	smartID := createEvalModel(t, ts.URL, stubSmart.URL, "cruxeval-smart")
	dumbID := createEvalModel(t, ts.URL, stubDumb.URL, "cruxeval-dumb")

	suite, cases := cruxevalSuiteCases(t, ts.URL)
	suiteID := int64(suite["id"].(float64))
	for i, c := range cases {
		stubSmart.setAnswerSeq(c.prompt, outputCorrectVariants[i%len(outputCorrectVariants)](c.expected))
		stubDumb.setAnswerSeq(c.prompt, outputWrongVariants(i, c.expected))
	}

	runID := triggerEval(t, ts.URL, suiteID, smartID, dumbID)
	run := waitEvalDone(t, ts.URL, runID)
	if run["status"] != "done" {
		t.Fatalf("run status = %v, want done", run["status"])
	}

	smart := resultsByModel(run, "cruxeval-smart")
	if len(smart) != 20 {
		t.Fatalf("smart model has %d results, want 20", len(smart))
	}
	for _, r := range smart {
		if r["score"] != 1.0 {
			t.Errorf("smart case %v score = %v, want 1 (answer %q, expected via case, detail: %v)",
				r["case_id"], r["score"], r["answer_text"], r["verdict_detail"])
		}
		detail := r["verdict_detail"].(string)
		if !strings.Contains(detail, "rule output_match matched") {
			t.Errorf("smart verdict_detail should explain the output_match hit: %v", detail)
		}
		if strings.Contains(detail, "judge") {
			t.Errorf("rule verdict must never involve the judge: %v", detail)
		}
	}
	// sample_count = 1: exactly one completion call per case.
	for _, c := range cases[:5] {
		if calls := stubSmart.callCount("cruxeval-smart", c.prompt); calls != 1 {
			t.Errorf("case %d answered %d times, want 1 (sample_count=1)", c.id, calls)
		}
	}

	dumb := resultsByModel(run, "cruxeval-dumb")
	if len(dumb) != 20 {
		t.Fatalf("dumb model has %d results, want 20", len(dumb))
	}
	for _, r := range dumb {
		if r["score"] != 0.0 {
			t.Errorf("dumb case %v score = %v, want 0 (answer %q, detail: %v)",
				r["case_id"], r["score"], r["answer_text"], r["verdict_detail"])
		}
		detail := r["verdict_detail"].(string)
		if !strings.Contains(detail, "rule output_match") || strings.Contains(detail, "matched") {
			t.Errorf("dumb verdict_detail should report the output_match miss: %v", detail)
		}
	}

	// Aggregate: raw mean (100 x 1 + 100 x 0) / 200 = 0.5, nadir 0 keeps the
	// normalized score at 0.5.
	if score, ok := run["score"].(float64); !ok || score != 0.5 {
		t.Errorf("run score = %v, want 0.5 (nadir 0, raw mean 0.5)", run["score"])
	}
}

// TestCRUXEvalCampaignReportShowsCoding runs a campaign containing the
// cruxeval suite and asserts the report carries the coding-dimension score.
func TestCRUXEvalCampaignReportShowsCoding(t *testing.T) {
	ts, stub, _ := setupEvalEnv(t)
	modelID := createEvalModel(t, ts.URL, stub.URL, "cruxeval-ace")

	suite, cases := cruxevalSuiteCases(t, ts.URL)
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
		if s.(map[string]interface{})["key"] == "cruxeval" {
			found = true
		}
	}
	if !found {
		t.Errorf("report suites should cover cruxeval: %v", suites)
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
	if scores["cruxeval"] != 100.0 {
		t.Errorf("coding dimension (cruxeval) score = %v, want 20 (all correct)", scores["cruxeval"])
	}
	if row["total_score"] != 100.0 {
		t.Errorf("total_score = %v, want 20", row["total_score"])
	}
}

// TestCRUXEvalOutputMatchCaseValidation covers the admin API boundary for
// output_match cases: the expectation must be a parseable Python literal,
// anything else could never score a hit and is rejected at creation.
func TestCRUXEvalOutputMatchCaseValidation(t *testing.T) {
	ts, _, _ := setupEvalEnv(t)
	suiteID := suiteIDByKey(t, ts.URL, "cruxeval")

	create := func(expected string) int {
		resp := doPost(t, ts.URL+"/api/cases", map[string]interface{}{
			"suite_id":     suiteID,
			"prompt":       "def f(n):\n    return n\nassert f(1) == ??",
			"verdict_type": "rule",
			"rule_config":  map[string]string{"mode": "output_match", "expected": expected},
		})
		defer resp.Body.Close()
		return resp.StatusCode
	}

	for _, bad := range []string{"", "hello world", "1 +", "f(1)", "[1, 2", "{'a': }"} {
		if code := create(bad); code != 400 {
			t.Errorf("output_match expected %q: got %d, want 400", bad, code)
		}
	}
	for _, good := range []string{"[1, 2]", "'a'", `"a"`, "True", "{'a': 1}", "(4,)", "3.5", "-2"} {
		if code := create(good); code != 201 {
			t.Errorf("output_match expected %q: got %d, want 201", good, code)
		}
	}
}
