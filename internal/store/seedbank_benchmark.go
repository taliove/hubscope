package store

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
)

// benchmarkSuites is the authoritative-benchmark seed bank (spec 0014
// decision C, ADR 0013): frozen subsets of public benchmarks shipped as
// embedded data files (W8) and cast into suites by the same
// generation-idempotent mechanism as the v3 bank. Each suite here is the
// template tickets 95-98 replicate: one embedded JSONL subset, one
// ATTRIBUTION file beside it, one entry in this list.
//
// Every benchmark suite seeds DISABLED via retireAtGen 1: the benchmark bank
// replaces the v3 self-written suites only at the deliberate cutover
// (ticket 99), so until then the new suites stay out of full sweeps and the
// weekly batch while manual triggers and admin re-enable stay available.
// The generation record makes an admin re-enable sticky across restarts.
//
//go:embed benchmark/mmlu/subset.jsonl
var mmluSubset string

//go:embed benchmark/gsm8k/subset.jsonl
var gsm8kSubset string

//go:embed benchmark/agieval_zh/subset.jsonl
var agievalZhSubset string

//go:embed benchmark/cruxeval/subset.jsonl
var cruxevalSubset string

var benchmarkSuites = []seedSuite{
	mustMCQSuite(mcqSuiteSpec{
		key:        "mmlu",
		name:       "知识（MMLU）",
		capability: CapabilityKnowledge,
		// Four-option multiple choice throughout: the random-guess floor is
		// 0.25, same caliber as cap_knowledge. Ticket 99 recalibrates nadirs
		// from the observed score distribution before the cutover.
		nadir:          0.25,
		data:           mmluSubset,
		promptTemplate: mcqPromptTemplate,
	}),
	mustMCQSuite(mcqSuiteSpec{
		key:        "agieval_zh",
		name:       "中文（AGIEval）",
		capability: CapabilityLanguage,
		// Four-option Gaokao multiple choice throughout: the random-guess
		// floor is 0.25, same caliber as mmlu. Ticket 99 recalibrates nadirs
		// from the observed score distribution before the cutover.
		nadir:          0.25,
		data:           agievalZhSubset,
		promptTemplate: mcqPromptTemplateZh,
	}),
	mustNumericSuite(numericSuiteSpec{
		key:        "gsm8k",
		name:       "推理（GSM8K）",
		capability: CapabilityReasoning,
		// Open-ended numeric answers have no meaningful random-guess floor;
		// nadir stays 0. Ticket 99 recalibrates nadirs from the observed
		// score distribution before the cutover.
		nadir: 0,
		data:  gsm8kSubset,
	}),
	mustCruxEvalSuite(cruxevalSuiteSpec{
		key:        "cruxeval",
		name:       "代码（CRUXEval）",
		capability: CapabilityCoding,
		// Output prediction has no random-guess floor; nadir stays 0 until
		// ticket 99 recalibrates from the observed score distribution.
		nadir: 0,
		data:  cruxevalSubset,
	}),
}

// mcqSuiteSpec describes one multiple-choice benchmark suite to seed.
type mcqSuiteSpec struct {
	key            string
	name           string
	capability     string
	nadir          float64
	data           string // embedded JSONL subset
	promptTemplate string // frozen into every case at seed time (W7)
}

// mcqQuestion is one row of an embedded multiple-choice subset file: the
// question, its four choices, and the correct option letter. The subject
// field documents the stratification of the frozen subset (see the
// ATTRIBUTION file beside the data); it is metadata, not cast into cases.
// passage is optional reading material (AGIEval gaokao-chinese); when
// present it is composed into the prompt before the question.
type mcqQuestion struct {
	ID       string   `json:"id"`
	Subject  string   `json:"subject"`
	Question string   `json:"question"`
	Choices  []string `json:"choices"`
	Answer   string   `json:"answer"`
	Passage  string   `json:"passage,omitempty"`
}

// mcqPromptTemplate is the prompt every English MCQ benchmark case is cast
// with (ADR 0013): the model is asked for exactly one option letter, and the
// mcq rule verdict extracts that letter from its reply. The template is frozen
// into each case at seed time — changing it means retiring the suite and
// casting a new one (W7).
const mcqPromptTemplate = "The following is a multiple-choice question. Reply with only the letter of the correct option (A, B, C, or D).\n\n%s\nA. %s\nB. %s\nC. %s\nD. %s"

// mcqPromptTemplateZh is the Chinese counterpart cast into the agieval_zh
// suite (ticket 96): same single-letter contract, frozen the same way. The
// mcq rule verdict's extraction patterns already cover the Chinese answer
// idioms this template invites (「答案是B」「选B」, full-width letters via
// NFKC), so both suites share one scoring caliber.
const mcqPromptTemplateZh = "以下是一道单项选择题，请只回复正确选项的字母（A、B、C 或 D）。\n\n%s\nA. %s\nB. %s\nC. %s\nD. %s"

// mustMCQSuite parses an embedded MCQ subset and builds its seedSuite. The
// data is compiled into the binary, so a malformed file is a build-time bug
// and panics at init rather than failing at runtime. Cases carry generation
// 1: each benchmark suite tracks its own seed generation lineage.
func mustMCQSuite(spec mcqSuiteSpec) seedSuite {
	if spec.promptTemplate == "" {
		panic(fmt.Sprintf("benchmark %s: missing prompt template", spec.key))
	}
	suite := seedSuite{
		key:         spec.key,
		name:        spec.name,
		capability:  spec.capability,
		nadir:       spec.nadir,
		retireAtGen: 1, // seeds disabled pre-cutover; see benchmarkSuites comment
	}
	seen := map[string]bool{}
	for lineNo, line := range strings.Split(spec.data, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var q mcqQuestion
		if err := json.Unmarshal([]byte(line), &q); err != nil {
			panic(fmt.Sprintf("benchmark %s: line %d: invalid JSON: %v", spec.key, lineNo+1, err))
		}
		if q.ID == "" || seen[q.ID] {
			panic(fmt.Sprintf("benchmark %s: line %d: missing or duplicate id %q", spec.key, lineNo+1, q.ID))
		}
		seen[q.ID] = true
		if len(q.Choices) != 4 {
			panic(fmt.Sprintf("benchmark %s: %s: %d choices, want 4", spec.key, q.ID, len(q.Choices)))
		}
		if len(q.Answer) != 1 || !strings.Contains("ABCD", q.Answer) {
			panic(fmt.Sprintf("benchmark %s: %s: answer %q, want one of A-D", spec.key, q.ID, q.Answer))
		}
		// Reading material precedes the question (AGIEval gaokao-chinese);
		// the composition is part of the frozen prompt.
		body := q.Question
		if q.Passage != "" {
			body = q.Passage + "\n\n" + q.Question
		}
		suite.cases = append(suite.cases, seedCase{
			gen: 1,
			// The source carries no per-item difficulty labels; the tier is
			// a neutral placeholder, not a measurement.
			difficulty:   "intermediate",
			sampleCount:  intptr(1),
			prompt:       fmt.Sprintf(spec.promptTemplate, body, q.Choices[0], q.Choices[1], q.Choices[2], q.Choices[3]),
			verdictType:  "rule",
			ruleMode:     strptr("mcq"),
			ruleExpected: strptr(q.Answer),
		})
	}
	if len(suite.cases) == 0 {
		panic(fmt.Sprintf("benchmark %s: empty subset", spec.key))
	}
	return suite
}

// numericSuiteSpec describes one numeric-answer benchmark suite to seed.
type numericSuiteSpec struct {
	key        string
	name       string
	capability string
	nadir      float64
	data       string // embedded JSONL subset
}

// numericQuestion is one row of an embedded numeric-answer subset file: the
// question and its expected final numeric answer (canonical form: no
// thousands separators). The source_index field records the upstream row
// index for audit (see the ATTRIBUTION file beside the data); it is
// metadata, not cast into cases.
type numericQuestion struct {
	ID          string `json:"id"`
	SourceIndex int    `json:"source_index"`
	Question    string `json:"question"`
	Answer      string `json:"answer"`
}

// numericPromptTemplate is the prompt every GSM8K-style benchmark case is
// cast with (ADR 0013): the model is asked to end its reply with the
// official '#### N' marker, and the numeric rule verdict extracts that
// final number from its reply. The template is frozen into each case at
// seed time — changing it means retiring the suite and casting a new one
// (W7).
const numericPromptTemplate = "Solve the following grade school math word problem. Think step by step, then end your reply with a line containing only '#### N', where N is the final numeric answer.\n\n%s"

// mustNumericSuite parses an embedded numeric subset and builds its
// seedSuite, under the same compile-time-data contract as mustMCQSuite.
func mustNumericSuite(spec numericSuiteSpec) seedSuite {
	suite := seedSuite{
		key:         spec.key,
		name:        spec.name,
		capability:  spec.capability,
		nadir:       spec.nadir,
		retireAtGen: 1, // seeds disabled pre-cutover; see benchmarkSuites comment
	}
	seen := map[string]bool{}
	for lineNo, line := range strings.Split(spec.data, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var q numericQuestion
		if err := json.Unmarshal([]byte(line), &q); err != nil {
			panic(fmt.Sprintf("benchmark %s: line %d: invalid JSON: %v", spec.key, lineNo+1, err))
		}
		if q.ID == "" || seen[q.ID] {
			panic(fmt.Sprintf("benchmark %s: line %d: missing or duplicate id %q", spec.key, lineNo+1, q.ID))
		}
		seen[q.ID] = true
		if q.Question == "" || q.Answer == "" {
			panic(fmt.Sprintf("benchmark %s: %s: empty question or answer", spec.key, q.ID))
		}
		suite.cases = append(suite.cases, seedCase{
			gen: 1,
			// The source carries no per-item difficulty labels; the tier is
			// a neutral placeholder, not a measurement.
			difficulty:   "intermediate",
			sampleCount:  intptr(1),
			prompt:       fmt.Sprintf(numericPromptTemplate, q.Question),
			verdictType:  "rule",
			ruleMode:     strptr("numeric"),
			ruleExpected: strptr(q.Answer),
		})
	}
	if len(suite.cases) == 0 {
		panic(fmt.Sprintf("benchmark %s: empty subset", spec.key))
	}
	return suite
}

// cruxevalSuiteSpec describes one CRUXEval-style output-prediction benchmark
// suite to seed.
type cruxevalSuiteSpec struct {
	key        string
	name       string
	capability string
	nadir      float64
	data       string // embedded JSONL subset
}

// cruxevalQuestion is one row of the embedded output-prediction subset: the
// function under test, the call input, and the standard output precomputed
// offline (scripts/cruxeval_subset.py). source_id documents the upstream row
// (see the ATTRIBUTION file beside the data); it is metadata, not cast into
// cases.
type cruxevalQuestion struct {
	ID       string `json:"id"`
	SourceID string `json:"source_id"`
	Code     string `json:"code"`
	Input    string `json:"input"`
	Output   string `json:"output"`
}

// cruxevalPromptTemplate is the prompt every output-prediction case is cast
// with (ticket 98): the official CRUXEval direct-output caliber adapted for
// chat models — predict the literal output, nothing else. The template is
// frozen into each case at seed time — changing it means retiring the suite
// and casting a new one (W7).
const cruxevalPromptTemplate = "You are given a Python function and an assertion containing an input to the function. Determine the output when executing the provided code on the given input, even if the function is incorrect or incomplete. Reply with only the output as a single Python literal (no unsimplified expressions, no function calls, no explanation, no code fences).\n\n%s\nassert f(%s) == ??"

// mustCruxEvalSuite parses the embedded CRUXEval subset and builds its
// seedSuite. The data is compiled into the binary, so a malformed file is a
// build-time bug and panics at init rather than failing at runtime. The
// standard answers were verified offline against actual execution by the
// authoring script, so the seed only checks structural invariants; the
// output_match rule validates literals again at case-creation time.
func mustCruxEvalSuite(spec cruxevalSuiteSpec) seedSuite {
	suite := seedSuite{
		key:         spec.key,
		name:        spec.name,
		capability:  spec.capability,
		nadir:       spec.nadir,
		retireAtGen: 1, // seeds disabled pre-cutover; see benchmarkSuites comment
	}
	seen := map[string]bool{}
	for lineNo, line := range strings.Split(spec.data, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var q cruxevalQuestion
		if err := json.Unmarshal([]byte(line), &q); err != nil {
			panic(fmt.Sprintf("benchmark %s: line %d: invalid JSON: %v", spec.key, lineNo+1, err))
		}
		if q.ID == "" || seen[q.ID] {
			panic(fmt.Sprintf("benchmark %s: line %d: missing or duplicate id %q", spec.key, lineNo+1, q.ID))
		}
		seen[q.ID] = true
		if q.Code == "" || q.Input == "" || q.Output == "" {
			panic(fmt.Sprintf("benchmark %s: %s: code/input/output must all be non-empty", spec.key, q.ID))
		}
		suite.cases = append(suite.cases, seedCase{
			gen: 1,
			// The source carries no per-item difficulty labels; the tier is
			// a neutral placeholder, not a measurement.
			difficulty:   "intermediate",
			sampleCount:  intptr(1),
			prompt:       fmt.Sprintf(cruxevalPromptTemplate, q.Code, q.Input),
			verdictType:  "rule",
			ruleMode:     strptr("output_match"),
			ruleExpected: strptr(q.Output),
		})
	}
	if len(suite.cases) == 0 {
		panic(fmt.Sprintf("benchmark %s: empty subset", spec.key))
	}
	return suite
}
