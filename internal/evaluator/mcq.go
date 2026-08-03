package evaluator

import (
	"fmt"
	"regexp"
	"strings"

	"golang.org/x/text/unicode/norm"
)

// mcqVerdict scores a multiple-choice answer (spec 0014 decision C, ADR
// 0013): extract the single option letter the model committed to and compare
// it exactly with the case's expected letter. Extraction is conservative by
// design — no letter or conflicting letters means a miss (0), never a guess.
// A miss is still a scored verdict (the model did answer), so the score is
// never nil; only an answer-call failure keeps the case unscored (W7).
//
// Extraction caliber, in layer order (see extractOptionLetter):
//  1. Explicit phrasing patterns over the NFKC-folded, whitespace-collapsed
//     answer ("the answer is B", 答案是B, 选B, a bare letter as the whole
//     answer). Exactly one distinct letter across every capture counts.
//  2. Last-line fallback (field report 2026-08-03: a long timezone-
//     conversion reasoning whose final line was a lone "C" scored 0): the
//     last non-empty line of the raw answer, when the whole line is exactly
//     one option letter plus at most one trailing punctuation mark, is a
//     commitment to that letter.
//  3. Otherwise no extraction — conservative beats clever, a miss is
//     preferable to a guessed hit (ADR 0013 宁缺毋猜).
func mcqVerdict(prompt, expected, answer string) (*float64, string) {
	want := strings.ToUpper(strings.TrimSpace(expected))
	letter, found := extractOptionLetter(prompt, expected, answer)
	if !found {
		return scorePtr(0), fmt.Sprintf("rule mcq: no unambiguous option letter extracted (expected %q)", expected)
	}
	if letter == want {
		return scorePtr(1), fmt.Sprintf("rule mcq matched (expected %q)", expected)
	}
	return scorePtr(0), fmt.Sprintf("rule mcq answered %q (expected %q)", letter, expected)
}

// mcqPatterns extract an option letter from phrasings models actually use.
// They run on the NFKC-folded, whitespace-collapsed answer (the ADR 0008 v2
// pipeline), case-insensitively. Order does not matter: matches from every
// pattern are collected and only a single distinct letter across all of them
// counts as an extraction.
var mcqPatterns = []*regexp.Regexp{
	// "The answer is B", "answer is: (B)"
	regexp.MustCompile(`(?i)\banswers?\s+is\s*:?\s*\(?([A-D])\)?`),
	// 答案B / 答案是B / 答案为B / 答案：B / 答案是 (B)
	regexp.MustCompile(`(?i)答案\s*(?:是|为|:|：)?\s*\(?([A-D])\)?`),
	// "option B" / "choice: (B)"
	regexp.MustCompile(`(?i)\b(?:option|choice)\s*:?\s*\(?([A-D])\)?`),
	// 选B / 选择B / 选 (B)
	regexp.MustCompile(`(?i)选(?:择)?\s*\(?([A-D])\)?`),
	// The whole answer is just a letter with optional parens and one
	// trailing punctuation mark: "B", "b", "(B)", "B.", "b。"
	regexp.MustCompile(`(?i)^\(?([A-D])\)?\s*[.。、,，:：]?\s*$`),
	// The answer leads with a letter marker followed by more prose:
	// "B. Because...", "(B), I think", "B、因为……"
	regexp.MustCompile(`(?i)^\(?([A-D])\)?\s*[.。、,，:：]\s*\S`),
}

// mcqOptionMarkerRe spots option labels in a case prompt ("A. ", "B) ",
// "C、", "D: …"), so the candidate letter set is inferred from the question
// instead of being hardcoded. It runs on the NFKC-folded prompt, which maps
// full-width letters, parens and colons onto their ASCII forms.
var mcqOptionMarkerRe = regexp.MustCompile(`(?im)^\s*\(?([A-Za-z])\s*[\).、:]`)

// mcqLastLineRe is the entire layer-2 caliber: a whole line that is exactly
// one option letter with at most one trailing punctuation mark (。 or . —
// NFKC has already folded the full-width ． onto "."). Letters inside words
// (长征5号B) can never match because the line must contain nothing else.
var mcqLastLineRe = regexp.MustCompile(`^([A-Za-z])[.。]?$`)

// extractOptionLetter pulls the single option letter a model committed to
// out of its answer, applying the layered caliber documented on mcqVerdict.
// Layer 1 folds the answer through the v2 normalization pipeline (NFKC maps
// full-width letters onto ASCII) and collects every phrasing pattern's
// captures case-insensitively: exactly one distinct letter is an extraction,
// zero or conflicting letters are not. Only then does layer 2 consider the
// raw answer's last non-empty line. There is no "last letter anywhere" rule
// and never will be — that is the aggressive misreading ADR 0013 forbids.
func extractOptionLetter(prompt, expected, answer string) (string, bool) {
	s := normalizeV2(answer)
	found := map[byte]bool{}
	consider := func(raw string) {
		up := strings.ToUpper(raw)
		if len(up) == 1 && up[0] >= 'A' && up[0] <= 'D' {
			found[up[0]] = true
		}
	}
	for _, re := range mcqPatterns {
		for _, m := range re.FindAllStringSubmatch(s, -1) {
			consider(m[1])
		}
	}
	if len(found) == 1 {
		for letter := range found {
			return string(letter), true
		}
	}
	return lastLineOptionLetter(answer, mcqOptionSet(prompt, expected))
}

// mcqOptionSet infers the candidate option letters from the case prompt's
// option markers plus the expected letter, so the last-line fallback
// validates against the question's real options rather than a hardcoded
// A-D. When the prompt shows no markers, the API-validated mcq domain A-D
// (the case API rejects expectations outside it) is the default.
func mcqOptionSet(prompt, expected string) map[byte]bool {
	set := map[byte]bool{}
	want := strings.ToUpper(strings.TrimSpace(expected))
	if len(want) == 1 && want[0] >= 'A' && want[0] <= 'Z' {
		set[want[0]] = true
	}
	for _, m := range mcqOptionMarkerRe.FindAllStringSubmatch(norm.NFKC.String(prompt), -1) {
		up := strings.ToUpper(m[1])
		if len(up) == 1 && up[0] >= 'A' && up[0] <= 'Z' {
			set[up[0]] = true
		}
	}
	if len(set) <= 1 {
		for l := byte('A'); l <= 'D'; l++ {
			set[l] = true
		}
	}
	return set
}

// lastLineOptionLetter is the layer-2 fallback: walk the raw answer's lines
// from the bottom, take the first non-empty one, and accept it only when the
// whole line is a single candidate option letter with at most one trailing
// punctuation mark. Any other content on that line — extra letters, prose,
// a letter embedded in a word — is not an extraction.
func lastLineOptionLetter(answer string, options map[byte]bool) (string, bool) {
	lines := strings.Split(answer, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(norm.NFKC.String(lines[i]))
		if line == "" {
			continue
		}
		m := mcqLastLineRe.FindStringSubmatch(line)
		if m == nil {
			return "", false
		}
		up := strings.ToUpper(m[1])
		if options[up[0]] {
			return up, true
		}
		return "", false
	}
	return "", false
}
