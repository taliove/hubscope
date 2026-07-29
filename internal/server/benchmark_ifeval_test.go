package server_test

import (
	"fmt"
	"strings"
	"testing"
)

// Black-box coverage for the IFEval instruction-following suite (ticket 97,
// spec 0014 decision C, ADR 0013): the suite seeds from the embedded frozen
// subset with structured check params on each case, the ported instruction
// checkers score hit/miss per type end to end over a stub hub, multi-
// instruction prompts score all-or-nothing, and a campaign carrying the
// suite reports an instruction-dimension score. Zero LLM judge calls are
// involved anywhere in these scenarios.

// ifevalPortedTypes is the set of instruction ids the Go checker library
// ports (internal/evaluator/ifeval, 22 types). The seed is fail-closed: an
// unported type in the embedded data would panic at init and no test would
// boot — so asserting the seeded bank stays within this set is the seam-
// level observable of that guard. The three langdetect-dependent official
// types (language:response_language, change_case:english_capital,
// change_case:english_lowercase) are deliberately absent.
var ifevalPortedTypes = []string{
	"keywords:existence",
	"keywords:frequency",
	"keywords:forbidden_words",
	"keywords:letter_frequency",
	"length_constraints:number_sentences",
	"length_constraints:number_paragraphs",
	"length_constraints:number_words",
	"length_constraints:nth_paragraph_first_word",
	"detectable_content:number_placeholders",
	"detectable_content:postscript",
	"detectable_format:number_bullet_lists",
	"detectable_format:constrained_response",
	"detectable_format:number_highlighted_sections",
	"detectable_format:multiple_sections",
	"detectable_format:json_format",
	"detectable_format:title",
	"combination:two_responses",
	"combination:repeat_prompt",
	"startend:end_checker",
	"change_case:capital_word_frequency",
	"punctuation:no_comma",
	"startend:quotation",
}

// ifevalInstruction is one instruction parsed from a case's check_params.
type ifevalInstruction struct {
	id     string
	kwargs map[string]interface{}
}

// ifevalCaseFields extracts the fields these tests need from one case entry
// of the suite listing.
type ifevalCaseFields struct {
	id           int64
	prompt       string
	instructions []ifevalInstruction
}

// ifevalSuiteCases fetches the seeded ifeval suite and returns its cases
// with their parsed check params, failing on any shape drift.
func ifevalSuiteCases(t *testing.T, base string) (map[string]interface{}, []ifevalCaseFields) {
	t.Helper()
	suite := suiteByKey(t, base, "ifeval")
	raw, ok := suite["cases"].([]interface{})
	if !ok {
		t.Fatalf("ifeval suite cases missing: %v", suite)
	}
	cases := make([]ifevalCaseFields, 0, len(raw))
	for _, c := range raw {
		cm := c.(map[string]interface{})
		cp, ok := cm["check_params"].([]interface{})
		if !ok || len(cp) == 0 {
			t.Fatalf("ifeval case %v missing check_params: %v", cm["id"], cm)
		}
		var instructions []ifevalInstruction
		for _, item := range cp {
			im, ok := item.(map[string]interface{})
			if !ok {
				t.Fatalf("ifeval case %v malformed instruction entry: %v", cm["id"], item)
			}
			kwargs, _ := im["kwargs"].(map[string]interface{})
			instructions = append(instructions, ifevalInstruction{
				id:     im["instruction_id"].(string),
				kwargs: kwargs,
			})
		}
		cases = append(cases, ifevalCaseFields{
			id:           int64(cm["id"].(float64)),
			prompt:       cm["prompt"].(string),
			instructions: instructions,
		})
	}
	return suite, cases
}

// kwStr reads a string kwarg.
func kwStr(kw map[string]interface{}, key string) string {
	s, _ := kw[key].(string)
	return s
}

// kwNum reads a numeric kwarg.
func kwNum(kw map[string]interface{}, key string) int {
	f, _ := kw[key].(float64)
	return int(f)
}

// kwStrList reads a string-list kwarg.
func kwStrList(kw map[string]interface{}, key string) []string {
	raw, _ := kw[key].([]interface{})
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// passAnswer crafts a response that follows the given single instruction,
// per the official checker semantics. The frozen subset guarantees the
// kwargs shapes relied on here (plain-word keywords, thresholds >= 1 where
// a zero would trivialize the check).
func passAnswer(ins ifevalInstruction) string {
	kw := ins.kwargs
	switch ins.id {
	case "keywords:existence":
		return strings.Join(kwStrList(kw, "keywords"), " ")
	case "keywords:frequency":
		if kwStr(kw, "relation") == "at least" {
			return strings.Repeat(kwStr(kw, "keyword")+" ", kwNum(kw, "frequency"))
		}
		return "" // 0 occurrences satisfies any "less than N" with N >= 1
	case "keywords:forbidden_words":
		return "123 456 789" // digits never match \bword\b
	case "keywords:letter_frequency":
		if kwStr(kw, "let_relation") == "at least" {
			return strings.Repeat(kwStr(kw, "letter"), kwNum(kw, "let_frequency"))
		}
		return ""
	case "length_constraints:number_sentences":
		if kwStr(kw, "relation") == "at least" {
			return strings.TrimSpace(strings.Repeat("Sentence here. ", kwNum(kw, "num_sentences")))
		}
		return ""
	case "length_constraints:number_paragraphs":
		parts := make([]string, kwNum(kw, "num_paragraphs"))
		for i := range parts {
			parts[i] = "Paragraph text."
		}
		return strings.Join(parts, "\n***\n")
	case "length_constraints:number_words":
		if kwStr(kw, "relation") == "at least" {
			return strings.TrimSpace(strings.Repeat("word ", kwNum(kw, "num_words")))
		}
		return ""
	case "length_constraints:nth_paragraph_first_word":
		n := kwNum(kw, "num_paragraphs")
		nth := kwNum(kw, "nth_paragraph")
		parts := make([]string, n)
		for i := range parts {
			parts[i] = "filler begins."
		}
		parts[nth-1] = kwStr(kw, "first_word") + " begins."
		return strings.Join(parts, "\n\n")
	case "detectable_content:number_placeholders":
		return strings.TrimSpace(strings.Repeat("[item] ", kwNum(kw, "num_placeholders")))
	case "detectable_content:postscript":
		return "Some answer.\n" + kwStr(kw, "postscript_marker") + " extra note"
	case "detectable_format:number_bullet_lists":
		lines := make([]string, kwNum(kw, "num_bullets"))
		for i := range lines {
			lines[i] = fmt.Sprintf("* item %d", i+1)
		}
		return strings.Join(lines, "\n")
	case "detectable_format:constrained_response":
		return "My answer is yes."
	case "detectable_format:number_highlighted_sections":
		return strings.TrimSpace(strings.Repeat("*section one* ", kwNum(kw, "num_highlights")))
	case "detectable_format:multiple_sections":
		spliter := kwStr(kw, "section_spliter")
		var b strings.Builder
		for i := 1; i <= kwNum(kw, "num_sections"); i++ {
			fmt.Fprintf(&b, "%s %d\ncontent\n", spliter, i)
		}
		return b.String()
	case "detectable_format:json_format":
		return `{"key": "value"}`
	case "detectable_format:title":
		return "<<my answer title>>\nbody text"
	case "combination:two_responses":
		return "First response here ****** Second response here"
	case "combination:repeat_prompt":
		return kwStr(kw, "prompt_to_repeat") + " then the actual answer."
	case "startend:end_checker":
		return "Some answer. " + kwStr(kw, "end_phrase")
	case "change_case:capital_word_frequency":
		if kwStr(kw, "capital_relation") == "at least" {
			return strings.TrimSpace(strings.Repeat("CAPWORD ", kwNum(kw, "capital_frequency")))
		}
		return "all lowercase words"
	case "punctuation:no_comma":
		return "no commas here at all"
	case "startend:quotation":
		return `"a quoted response"`
	default:
		panic("passAnswer: unported instruction type " + ins.id)
	}
}

// failAnswer crafts a response that violates the given single instruction.
func failAnswer(ins ifevalInstruction) string {
	kw := ins.kwargs
	switch ins.id {
	case "keywords:existence":
		return ""
	case "keywords:frequency":
		if kwStr(kw, "relation") == "at least" {
			return "" // 0 occurrences misses any "at least N" with N >= 1
		}
		return strings.TrimSpace(strings.Repeat(kwStr(kw, "keyword")+" ", kwNum(kw, "frequency")))
	case "keywords:forbidden_words":
		return kwStrList(kw, "forbidden_words")[0]
	case "keywords:letter_frequency":
		if kwStr(kw, "let_relation") == "at least" {
			return ""
		}
		return strings.Repeat(kwStr(kw, "letter"), kwNum(kw, "let_frequency"))
	case "length_constraints:number_sentences":
		if kwStr(kw, "relation") == "at least" {
			if kwNum(kw, "num_sentences") <= 1 {
				return "" // 0 sentences misses "at least 1"
			}
			return "Just one." // 1 sentence misses "at least N >= 2"
		}
		return strings.TrimSpace(strings.Repeat("Sentence here. ", kwNum(kw, "num_sentences")))
	case "length_constraints:number_paragraphs":
		n := kwNum(kw, "num_paragraphs") + 1 // one too many
		parts := make([]string, n)
		for i := range parts {
			parts[i] = "Paragraph text."
		}
		return strings.Join(parts, "\n***\n")
	case "length_constraints:number_words":
		if kwStr(kw, "relation") == "at least" {
			return ""
		}
		return strings.TrimSpace(strings.Repeat("word ", kwNum(kw, "num_words")))
	case "length_constraints:nth_paragraph_first_word":
		n := kwNum(kw, "num_paragraphs")
		nth := kwNum(kw, "nth_paragraph")
		parts := make([]string, n)
		for i := range parts {
			parts[i] = "filler begins."
		}
		parts[nth-1] = "zzzwrong begins." // dataset first_words are real words
		return strings.Join(parts, "\n\n")
	case "detectable_content:number_placeholders":
		return "no placeholders"
	case "detectable_content:postscript":
		return "no postscript here"
	case "detectable_format:number_bullet_lists":
		return "no bullets here"
	case "detectable_format:constrained_response":
		return "I think so"
	case "detectable_format:number_highlighted_sections":
		return "no highlights"
	case "detectable_format:multiple_sections":
		return "no numbered markers here"
	case "detectable_format:json_format":
		return "not json"
	case "detectable_format:title":
		return "no title"
	case "combination:two_responses":
		return "only one response"
	case "combination:repeat_prompt":
		return "answer without repeating"
	case "startend:end_checker":
		return "some answer."
	case "change_case:capital_word_frequency":
		if kwStr(kw, "capital_relation") == "at least" {
			return "all lowercase words here"
		}
		return strings.TrimSpace(strings.Repeat("CAPWORD ", kwNum(kw, "capital_frequency")))
	case "punctuation:no_comma":
		return "yes, commas"
	case "startend:quotation":
		return "unquoted text"
	default:
		panic("failAnswer: unported instruction type " + ins.id)
	}
}

// singleInstructionCases indexes the subset's single-instruction cases by
// instruction type and asserts every ported type is exercised by at least
// one of them (the subset builder guarantees this; the acceptance requires
// per-type hit/miss coverage end to end).
func singleInstructionCases(t *testing.T, cases []ifevalCaseFields) map[string][]ifevalCaseFields {
	t.Helper()
	byType := map[string][]ifevalCaseFields{}
	for _, c := range cases {
		if len(c.instructions) == 1 {
			byType[c.instructions[0].id] = append(byType[c.instructions[0].id], c)
		}
	}
	for _, typ := range ifevalPortedTypes {
		if len(byType[typ]) == 0 {
			t.Errorf("ported instruction type %q has no single-instruction case in the subset", typ)
		}
	}
	return byType
}

// TestIFEvalSuiteSeeded asserts the benchmark seed casts the ifeval suite
// from the embedded subset: capability instruction, in the rotation since
// the ticket-99 cutover,
// zero nadir floor (free-form generation has no random-guess rate), and 100
// single-sample rule cases whose check params stay within the ported
// instruction set.
func TestIFEvalSuiteSeeded(t *testing.T) {
	ts, _, _ := setupEvalEnv(t)

	suite, cases := ifevalSuiteCases(t, ts.URL)
	if suite["capability"] != "instruction" {
		t.Errorf("ifeval capability = %v, want instruction", suite["capability"])
	}
	// Post-cutover (ticket 99): the benchmark suite is the rotation.
	if suite["enabled"] != true {
		t.Errorf("ifeval enabled = %v, want true (post-cutover rotation)", suite["enabled"])
	}
	if suite["nadir"] != 0.0 {
		t.Errorf("ifeval nadir = %v, want 0 (no random-guess floor; ticket 99 recalibrates)", suite["nadir"])
	}
	if len(cases) != 100 {
		t.Fatalf("ifeval cases = %d, want 100 (frozen subset)", len(cases))
	}

	ported := map[string]bool{}
	for _, typ := range ifevalPortedTypes {
		ported[typ] = true
	}
	raw := suite["cases"].([]interface{})
	for i, c := range raw {
		cm := c.(map[string]interface{})
		if cm["verdict_type"] != "rule" {
			t.Errorf("case %d verdict_type = %v, want rule (zero LLM judge)", i, cm["verdict_type"])
		}
		rc := cm["rule_config"].(map[string]interface{})
		if rc["mode"] != "ifeval" {
			t.Errorf("case %d rule mode = %v, want ifeval", i, rc["mode"])
		}
		if cm["sample_count"] != 1.0 {
			t.Errorf("case %d sample_count = %v, want 1", i, cm["sample_count"])
		}
		if cm["enabled"] != true {
			t.Errorf("case %d enabled = %v, want true", i, cm["enabled"])
		}
		for _, ins := range cases[i].instructions {
			if !ported[ins.id] {
				t.Errorf("case %d carries unported instruction type %q (seed must be fail-closed)", i, ins.id)
			}
		}
	}
}

// TestIFEvalSeedIdempotent reopens the database and asserts the benchmark
// seed never re-casts cases and never reverts an admin edit.
func TestIFEvalSeedIdempotent(t *testing.T) {
	dbPath := t.TempDir() + "/ifeval.db"

	ts, db := openSuitesServer(t, dbPath)
	_, cases := ifevalSuiteCases(t, ts.URL)
	if len(cases) != 100 {
		t.Fatalf("first boot ifeval cases = %d, want 100", len(cases))
	}
	// Admin curation: disable one case in place.
	patchCase(t, ts.URL, cases[0].id, map[string]interface{}{"enabled": false})
	db.Close()

	ts2, db2 := openSuitesServer(t, dbPath)
	defer db2.Close()
	suite, cases2 := ifevalSuiteCases(t, ts2.URL)
	if len(cases2) != 100 {
		t.Errorf("second boot ifeval cases = %d, want 100 (no seed duplicates)", len(cases2))
	}
	if v := suite["version"]; v != 2.0 {
		t.Errorf("ifeval version after reopen = %v, want 2 (seed must not rebump)", v)
	}
	first := caseByID(t, suite, cases[0].id)
	if first["enabled"] != false {
		t.Errorf("admin disable reverted by reseed: %v", first)
	}

	db2.Close()
	ts3, db3 := openSuitesServer(t, dbPath)
	defer db3.Close()
	_, cases3 := ifevalSuiteCases(t, ts3.URL)
	if len(cases3) != 100 {
		t.Errorf("third boot ifeval cases = %d, want still 100", len(cases3))
	}
}

// TestIFEvalVerdictsPerInstructionType runs the ifeval suite against a model
// that follows every single-instruction prompt and a model that violates
// every one, asserting each ported instruction checker scores 1/0 end to
// end — with zero judge involvement. Multi-instruction cases are answered
// arbitrarily here; their all-or-nothing semantics have a dedicated test.
func TestIFEvalVerdictsPerInstructionType(t *testing.T) {
	ts, stubSmart, _ := setupEvalEnv(t)
	stubDumb := newEvalStubHub()
	t.Cleanup(stubDumb.Close)

	smartID := createEvalModel(t, ts.URL, stubSmart.URL, "ifeval-smart")
	dumbID := createEvalModel(t, ts.URL, stubDumb.URL, "ifeval-dumb")

	suite, cases := ifevalSuiteCases(t, ts.URL)
	suiteID := int64(suite["id"].(float64))
	byType := singleInstructionCases(t, cases)

	for _, c := range cases {
		if len(c.instructions) == 1 {
			stubSmart.setAnswerSeq(c.prompt, passAnswer(c.instructions[0]))
			stubDumb.setAnswerSeq(c.prompt, failAnswer(c.instructions[0]))
		} else {
			stubSmart.setAnswerSeq(c.prompt, "unrelated answer")
			stubDumb.setAnswerSeq(c.prompt, "unrelated answer")
		}
	}

	runID := triggerEval(t, ts.URL, suiteID, smartID, dumbID)
	run := waitEvalDone(t, ts.URL, runID)
	if run["status"] != "done" {
		t.Fatalf("run status = %v, want done", run["status"])
	}

	singleByID := map[int64]ifevalCaseFields{}
	for _, singles := range byType {
		for _, c := range singles {
			singleByID[c.id] = c
		}
	}

	smart := resultsByModel(run, "ifeval-smart")
	if len(smart) != 100 {
		t.Fatalf("smart model has %d results, want 100", len(smart))
	}
	assertedSmart := 0
	for _, r := range smart {
		c, single := singleByID[int64(r["case_id"].(float64))]
		if !single {
			continue
		}
		assertedSmart++
		if r["score"] != 1.0 {
			t.Errorf("smart case %v (%s) score = %v, want 1 (answer %q, detail: %v)",
				r["case_id"], c.instructions[0].id, r["score"], r["answer_text"], r["verdict_detail"])
		}
		detail := r["verdict_detail"].(string)
		if !strings.Contains(detail, "rule ifeval: all 1 instructions followed") {
			t.Errorf("smart verdict_detail should report all instructions followed: %v", detail)
		}
		if strings.Contains(detail, "judge") {
			t.Errorf("rule verdict must never involve the judge: %v", detail)
		}
	}
	if assertedSmart != len(singleByID) {
		t.Errorf("asserted %d smart single-instruction results, want %d", assertedSmart, len(singleByID))
	}

	dumb := resultsByModel(run, "ifeval-dumb")
	if len(dumb) != 100 {
		t.Fatalf("dumb model has %d results, want 100", len(dumb))
	}
	for _, r := range dumb {
		c, single := singleByID[int64(r["case_id"].(float64))]
		if !single {
			continue
		}
		if r["score"] != 0.0 {
			t.Errorf("dumb case %v (%s) score = %v, want 0 (answer %q, detail: %v)",
				r["case_id"], c.instructions[0].id, r["score"], r["answer_text"], r["verdict_detail"])
		}
		detail := r["verdict_detail"].(string)
		if !strings.Contains(detail, "failed:") || !strings.Contains(detail, c.instructions[0].id) {
			t.Errorf("dumb verdict_detail should name the failed instruction: %v", detail)
		}
	}

	// sample_count = 1: exactly one completion call per case (spot check).
	for _, singles := range byType {
		c := singles[0]
		if calls := stubSmart.callCount("ifeval-smart", c.prompt); calls != 1 {
			t.Errorf("case %d answered %d times, want 1 (sample_count=1)", c.id, calls)
		}
	}
}

// TestIFEvalMultiInstructionAllOrNothing pins the IFEval scoring semantic:
// a prompt carrying several instructions scores 1 only when every one of
// them is followed; any single failure scores 0. The subset's
// keywords:existence + punctuation:no_comma case (ifeval-0029, source key
// 1508) is the vehicle: three models pass both, pass only the first, and
// pass only the second.
func TestIFEvalMultiInstructionAllOrNothing(t *testing.T) {
	ts, stub, _ := setupEvalEnv(t)

	_, cases := ifevalSuiteCases(t, ts.URL)
	var target *ifevalCaseFields
	for i, c := range cases {
		if len(c.instructions) == 2 &&
			c.instructions[0].id == "keywords:existence" &&
			c.instructions[1].id == "punctuation:no_comma" {
			target = &cases[i]
			break
		}
	}
	if target == nil {
		t.Fatalf("subset lost its keywords:existence + punctuation:no_comma two-instruction case")
	}
	keywords := kwStrList(target.instructions[0].kwargs, "keywords")

	bothID := createEvalModel(t, ts.URL, stub.URL, "ifeval-both")
	stubBothAnswer := strings.Join(keywords, " and ") + " appear here" // keywords, no comma
	stub.setAnswerSeq(target.prompt, stubBothAnswer)

	stubFirst := newEvalStubHub()
	t.Cleanup(stubFirst.Close)
	firstID := createEvalModel(t, ts.URL, stubFirst.URL, "ifeval-first-only")
	stubFirst.setAnswerSeq(target.prompt, strings.Join(keywords, ", ")+", with a comma") // keywords + comma

	stubSecond := newEvalStubHub()
	t.Cleanup(stubSecond.Close)
	secondID := createEvalModel(t, ts.URL, stubSecond.URL, "ifeval-second-only")
	stubSecond.setAnswerSeq(target.prompt, "nothing relevant here") // no keywords, no comma

	suite := suiteByKey(t, ts.URL, "ifeval")
	suiteID := int64(suite["id"].(float64))
	runID := triggerEval(t, ts.URL, suiteID, bothID, firstID, secondID)
	run := waitEvalDone(t, ts.URL, runID)
	if run["status"] != "done" {
		t.Fatalf("run status = %v, want done", run["status"])
	}

	scoreOf := func(modelID string) (interface{}, string) {
		for _, r := range resultsByModel(run, modelID) {
			if int64(r["case_id"].(float64)) == target.id {
				detail, _ := r["verdict_detail"].(string)
				return r["score"], detail
			}
		}
		t.Fatalf("model %s has no result for case %d", modelID, target.id)
		return nil, ""
	}

	score, detail := scoreOf("ifeval-both")
	if score != 1.0 {
		t.Errorf("both-followed score = %v, want 1 (detail: %v)", score, detail)
	}
	if !strings.Contains(detail, "all 2 instructions followed") {
		t.Errorf("both-followed detail should report all 2 followed: %v", detail)
	}

	score, detail = scoreOf("ifeval-first-only")
	if score != 0.0 {
		t.Errorf("first-only score = %v, want 0 (one instruction failed)", score)
	}
	if !strings.Contains(detail, "punctuation:no_comma") {
		t.Errorf("first-only detail should name the failed instruction: %v", detail)
	}

	score, detail = scoreOf("ifeval-second-only")
	if score != 0.0 {
		t.Errorf("second-only score = %v, want 0 (one instruction failed)", score)
	}
	if !strings.Contains(detail, "keywords:existence") {
		t.Errorf("second-only detail should name the failed instruction: %v", detail)
	}
}

// TestIFEvalCampaignReportShowsInstruction runs a campaign containing the
// ifeval suite and asserts the report carries the instruction-dimension
// score.
func TestIFEvalCampaignReportShowsInstruction(t *testing.T) {
	ts, stub, _ := setupEvalEnv(t)
	modelID := createEvalModel(t, ts.URL, stub.URL, "ifeval-ace")

	suite, cases := ifevalSuiteCases(t, ts.URL)
	suiteID := int64(suite["id"].(float64))
	for _, c := range cases {
		if len(c.instructions) == 1 {
			stub.setAnswerSeq(c.prompt, passAnswer(c.instructions[0]))
		} else {
			stub.setAnswerSeq(c.prompt, "unrelated answer")
		}
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
		if s.(map[string]interface{})["key"] == "ifeval" {
			found = true
		}
	}
	if !found {
		t.Errorf("report suites should cover ifeval: %v", suites)
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
	score, ok := scores["ifeval"].(float64)
	if !ok {
		t.Fatalf("instruction dimension (ifeval) score missing: %v", scores)
	}
	// Every single-instruction case passes; the exact value depends on how
	// the arbitrary multi-instruction answers score, so assert a strict
	// lower bound from the guaranteed passes (nadir 0 keeps the raw mean).
	singleCount := 0
	for _, c := range cases {
		if len(c.instructions) == 1 {
			singleCount++
		}
	}
	if score < float64(singleCount) || score > 100.0 {
		t.Errorf("ifeval score = %v, want within [%d, 100]", score, singleCount)
	}
	if row["total_score"] != score {
		t.Errorf("total_score = %v, want %v (single-suite campaign)", row["total_score"], score)
	}
}

// TestIFEvalCaseAdminBoundaries covers the admin API boundary for ifeval
// cases: the check params are seed-cast structured data the admin API cannot
// author, so creating an ifeval-mode case is rejected (fail-closed), while
// editing an existing ifeval case preserves its check params across the
// immutable retire-and-mint replace (ticket 97 acceptance).
func TestIFEvalCaseAdminBoundaries(t *testing.T) {
	ts, _, _ := setupEvalEnv(t)
	suiteID := suiteIDByKey(t, ts.URL, "ifeval")

	resp := doPost(t, ts.URL+"/api/cases", map[string]interface{}{
		"suite_id":     suiteID,
		"prompt":       "Reply without commas.",
		"verdict_type": "rule",
		"rule_config":  map[string]string{"mode": "ifeval", "expected": ""},
	})
	defer resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Errorf("creating an ifeval case without seeded check_params: got %d, want 400", resp.StatusCode)
	}

	// Editing an ifeval case's prompt retires and re-mints it (W7); the new
	// case must carry the same check params.
	_, cases := ifevalSuiteCases(t, ts.URL)
	target := cases[0]
	updated := patchCase(t, ts.URL, target.id, map[string]interface{}{
		"prompt": target.prompt + " (edited)",
	})
	newID := int64(updated["id"].(float64))
	if newID == target.id {
		t.Fatalf("content edit should mint a new case id, got same id %d", newID)
	}
	cp, ok := updated["check_params"].([]interface{})
	if !ok || len(cp) != len(target.instructions) {
		t.Fatalf("edited case lost its check_params: %v", updated)
	}
	for i, item := range cp {
		got := item.(map[string]interface{})["instruction_id"]
		if got != target.instructions[i].id {
			t.Errorf("edited case instruction %d = %v, want %q", i, got, target.instructions[i].id)
		}
	}
}
