package store

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/taliove/hubscope/internal/evaluator/ifeval"
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

//go:embed benchmark/ifeval/subset.jsonl
var ifevalSubset string

var benchmarkSuites = []seedSuite{
	mustMCQSuite(mcqSuiteSpec{
		key:        "mmlu",
		name:       "知识（MMLU）",
		capability: CapabilityKnowledge,
		// Four-option multiple choice throughout: the random-guess floor is
		// 0.25, same caliber as cap_knowledge. Ticket 99 recalibrates nadirs
		// from the observed score distribution before the cutover.
		nadir: 0.25,
		data:  mmluSubset,
	}),
	mustIFEvalSuite(ifevalSuiteSpec{
		key:        "ifeval",
		name:       "指令遵循（IFEval）",
		capability: CapabilityInstruction,
		// Free-form instruction following has no random-guess floor: nadir 0
		// keeps the raw-mean caliber until ticket 99 recalibrates from the
		// observed score distribution.
		nadir: 0,
		data:  ifevalSubset,
	}),
}

// mcqSuiteSpec describes one multiple-choice benchmark suite to seed.
type mcqSuiteSpec struct {
	key        string
	name       string
	capability string
	nadir      float64
	data       string // embedded JSONL subset
}

// mcqQuestion is one row of an embedded multiple-choice subset file: the
// question, its four choices, and the correct option letter. The subject
// field documents the stratification of the frozen subset (see the
// ATTRIBUTION file beside the data); it is metadata, not cast into cases.
type mcqQuestion struct {
	ID       string   `json:"id"`
	Subject  string   `json:"subject"`
	Question string   `json:"question"`
	Choices  []string `json:"choices"`
	Answer   string   `json:"answer"`
}

// mcqPromptTemplate is the prompt every MCQ benchmark case is cast with
// (ADR 0013): the model is asked for exactly one option letter, and the mcq
// rule verdict extracts that letter from its reply. The template is frozen
// into each case at seed time — changing it means retiring the suite and
// casting a new one (W7).
const mcqPromptTemplate = "The following is a multiple-choice question. Reply with only the letter of the correct option (A, B, C, or D).\n\n%s\nA. %s\nB. %s\nC. %s\nD. %s"

// mustMCQSuite parses an embedded MCQ subset and builds its seedSuite. The
// data is compiled into the binary, so a malformed file is a build-time bug
// and panics at init rather than failing at runtime. Cases carry generation
// 1: each benchmark suite tracks its own seed generation lineage.
func mustMCQSuite(spec mcqSuiteSpec) seedSuite {
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
		suite.cases = append(suite.cases, seedCase{
			gen: 1,
			// The source carries no per-item difficulty labels; the tier is
			// a neutral placeholder, not a measurement.
			difficulty:   "intermediate",
			sampleCount:  intptr(1),
			prompt:       fmt.Sprintf(mcqPromptTemplate, q.Question, q.Choices[0], q.Choices[1], q.Choices[2], q.Choices[3]),
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

// ifevalSuiteSpec describes one IFEval-style instruction-following benchmark
// suite to seed (ticket 97, spec 0014 decision C).
type ifevalSuiteSpec struct {
	key        string
	name       string
	capability string
	nadir      float64
	data       string // embedded JSONL subset
}

// ifevalQuestion is one row of the embedded IFEval subset file: the prompt
// plus its verifiable instructions (instruction id + kwargs), exactly as
// selected from the upstream dataset (see the ATTRIBUTION file beside the
// data). id/key document the frozen subset's lineage; they are metadata,
// not cast into cases.
type ifevalQuestion struct {
	ID           string `json:"id"`
	Key          int    `json:"key"`
	Prompt       string `json:"prompt"`
	Instructions []struct {
		InstructionID string         `json:"instruction_id"`
		Kwargs        map[string]any `json:"kwargs"`
	} `json:"instructions"`
}

// mustIFEvalSuite parses the embedded IFEval subset and builds its
// seedSuite. Every case carries its structured check parameters in
// check_params (the JSON array of {instruction_id, kwargs}) and scores via
// the "ifeval" rule mode (evaluator/ifeval_verdict.go). The seed is
// fail-closed like mustMCQSuite: a malformed file, a duplicate id, or an
// instruction type the checker library does not port panics at init —
// unportable types must never enter the bank (ticket 97 risk 2).
func mustIFEvalSuite(spec ifevalSuiteSpec) seedSuite {
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
		var q ifevalQuestion
		if err := json.Unmarshal([]byte(line), &q); err != nil {
			panic(fmt.Sprintf("benchmark %s: line %d: invalid JSON: %v", spec.key, lineNo+1, err))
		}
		if q.ID == "" || seen[q.ID] {
			panic(fmt.Sprintf("benchmark %s: line %d: missing or duplicate id %q", spec.key, lineNo+1, q.ID))
		}
		seen[q.ID] = true
		if q.Prompt == "" {
			panic(fmt.Sprintf("benchmark %s: %s: empty prompt", spec.key, q.ID))
		}
		if len(q.Instructions) == 0 {
			panic(fmt.Sprintf("benchmark %s: %s: no instructions", spec.key, q.ID))
		}
		// Marshal back to the canonical check_params form (sorted kwarg
		// keys, deterministic) and validate against the ported checker set:
		// unsupported instruction ids or out-of-range kwargs are build-time
		// bugs, never runtime surprises.
		checkParams, err := json.Marshal(q.Instructions)
		if err != nil {
			panic(fmt.Sprintf("benchmark %s: %s: marshal check_params: %v", spec.key, q.ID, err))
		}
		if err := ifeval.Validate(string(checkParams)); err != nil {
			panic(fmt.Sprintf("benchmark %s: %s: %v", spec.key, q.ID, err))
		}
		suite.cases = append(suite.cases, seedCase{
			gen: 1,
			// The source carries no per-item difficulty labels; the tier is
			// a neutral placeholder, not a measurement.
			difficulty:  "intermediate",
			sampleCount: intptr(1),
			prompt:      q.Prompt,
			verdictType: "rule",
			ruleMode:    strptr("ifeval"),
			checkParams: strptr(string(checkParams)),
		})
	}
	if len(suite.cases) == 0 {
		panic(fmt.Sprintf("benchmark %s: empty subset", spec.key))
	}
	return suite
}
