// Package ifeval is a deterministic Go port of the IFEval verifiable-
// instruction checkers (spec 0014 decision C): every checker replicates the
// official check_following implementation in google-research's
// instruction_following_eval/instructions.py (master, fetched 2026-07-28,
// repo revision pinned by the dataset card), with the official regex
// utilities of instructions_util.py where they apply. Each checker cites its
// official class; a porting deviation is a leaderboard-caliber deviation, so
// the two sanctioned approximations (both replacing nltk/langdetect
// machinery that cannot ship in the single binary) are documented where they
// live:
//
//  1. length_constraints:number_sentences — the official NumberOfSentences
//     counts with the nltk Punkt tokenizer (instructions_util.count_sentences).
//     The port counts with instructions_util.split_into_sentences, the official
//     deterministic regex splitter from the same file (used upstream by the
//     disabled KeySentenceChecker). Same repo, deterministic, no model data.
//  2. change_case:capital_word_frequency — the official
//     CapitalWordFrequencyChecker tokenizes with nltk.word_tokenize (Punkt +
//     Treebank). The port tokenizes on whitespace; for the all-caps test
//     (Python str.isupper semantics, ported exactly) the two tokenizers agree
//     except around contractions and intra-word punctuation.
//
// Three official instruction types depend on the langdetect language model
// (language:response_language, change_case:english_capital,
// change_case:english_lowercase) and are NOT ported: they cannot be made
// deterministic without shipping a language-detection dependency. The frozen
// subset excludes every prompt carrying them, and Validate rejects them
// fail-closed, so they can never enter the bank.
package ifeval

import (
	"encoding/json"
	"fmt"
	"sort"
)

// Instruction is one verifiable instruction of an IFEval prompt: the
// official instruction id plus its kwargs, as carried by the dataset's
// instruction_id_list / kwargs fields. It is the wire shape of a case's
// check_params column (a JSON array of Instruction).
type Instruction struct {
	ID     string `json:"instruction_id"`
	Kwargs kwargs `json:"kwargs"`
}

// kwargs is one instruction's parameter map as decoded from JSON (numbers
// arrive as float64).
type kwargs map[string]any

// str reads a string kwarg.
func (k kwargs) str(key string) (string, bool) {
	v, ok := k[key].(string)
	return v, ok
}

// num reads an integral numeric kwarg.
func (k kwargs) num(key string) (int, bool) {
	f, ok := k[key].(float64)
	if !ok || f != float64(int(f)) {
		return 0, false
	}
	return int(f), true
}

// strList reads a string-list kwarg.
func (k kwargs) strList(key string) ([]string, bool) {
	raw, ok := k[key].([]any)
	if !ok {
		return nil, false
	}
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		s, ok := v.(string)
		if !ok {
			return nil, false
		}
		out = append(out, s)
	}
	return out, true
}

// instructionSpec pairs a checker with the validator that guarantees its
// kwargs are complete and in range (mirroring the validations inside the
// official build_description methods, minus their random fallbacks — a
// randomized check is unmeasurable, so out-of-range kwargs are rejected
// fail-closed instead).
type instructionSpec struct {
	validate func(kw kwargs) error
	check    func(kw kwargs, response string) (bool, error)
}

// Supported reports whether an instruction id has a ported checker.
func Supported(id string) bool {
	_, ok := registry[id]
	return ok
}

// SupportedIDs returns the ported instruction ids, sorted.
func SupportedIDs() []string {
	ids := make([]string, 0, len(registry))
	for id := range registry {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// Parse decodes a check_params JSON array. Unknown instruction ids are
// rejected fail-closed (the mustIFEvalSuite precedent: an unportable type
// must panic at seed time, never score silently).
func Parse(data string) ([]Instruction, error) {
	var instructions []Instruction
	if err := json.Unmarshal([]byte(data), &instructions); err != nil {
		return nil, fmt.Errorf("invalid check_params JSON: %w", err)
	}
	if len(instructions) == 0 {
		return nil, fmt.Errorf("check_params must carry at least one instruction")
	}
	for i, ins := range instructions {
		if !Supported(ins.ID) {
			return nil, fmt.Errorf("instruction %d: unsupported instruction_id %q (not ported; must not enter the bank)", i, ins.ID)
		}
		if ins.Kwargs == nil {
			ins.Kwargs = kwargs{}
			instructions[i] = ins
		}
	}
	return instructions, nil
}

// Validate parses check_params and validates every instruction's kwargs
// against the official build_description constraints. It is the seed-time
// and admin-API guard; Check assumes validated input but re-checks
// defensively.
func Validate(data string) error {
	instructions, err := Parse(data)
	if err != nil {
		return err
	}
	for _, ins := range instructions {
		if err := registry[ins.ID].validate(ins.Kwargs); err != nil {
			return fmt.Errorf("%s: %w", ins.ID, err)
		}
	}
	return nil
}

// Check runs every instruction against the response and returns the ids
// that failed (empty = all followed). IFEval scoring is all-or-nothing: the
// prompt scores 1 only when every instruction is followed. Malformed
// check_params yield an error, never a score (W7: a broken caliber is not a
// model failure).
func Check(data, response string) (total int, failed []string, err error) {
	instructions, err := Parse(data)
	if err != nil {
		return 0, nil, err
	}
	for _, ins := range instructions {
		spec := registry[ins.ID]
		if verr := spec.validate(ins.Kwargs); verr != nil {
			return 0, nil, fmt.Errorf("%s: %w", ins.ID, verr)
		}
		ok, cerr := spec.check(ins.Kwargs, response)
		if cerr != nil {
			return 0, nil, fmt.Errorf("%s: %w", ins.ID, cerr)
		}
		if !ok {
			failed = append(failed, ins.ID)
		}
	}
	return len(instructions), failed, nil
}

// comparison relations of the official _COMPARISON_RELATION constant.
const (
	relationLessThan = "less than"
	relationAtLeast  = "at least"
)

// compareRelation applies the official two-way comparison: "less than"
// means actual < threshold, "at least" means actual >= threshold.
func compareRelation(relation string, actual, threshold int) (bool, error) {
	switch relation {
	case relationLessThan:
		return actual < threshold, nil
	case relationAtLeast:
		return actual >= threshold, nil
	default:
		return false, fmt.Errorf("relation must be %q or %q, got %q", relationLessThan, relationAtLeast, relation)
	}
}

// requireRelation validates a relation kwarg against _COMPARISON_RELATION.
func requireRelation(kw kwargs, key string) error {
	rel, ok := kw.str(key)
	if !ok || (rel != relationLessThan && rel != relationAtLeast) {
		return fmt.Errorf("kwargs.%s must be %q or %q", key, relationLessThan, relationAtLeast)
	}
	return nil
}

// requireNonNegInt validates an integral kwarg >= 0 (the official
// build_description methods randomize on None or negative; the port rejects
// instead).
func requireNonNegInt(kw kwargs, key string) error {
	n, ok := kw.num(key)
	if !ok || n < 0 {
		return fmt.Errorf("kwargs.%s must be an integer >= 0", key)
	}
	return nil
}
