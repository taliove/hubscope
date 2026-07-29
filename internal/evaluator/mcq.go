package evaluator

import (
	"fmt"
	"regexp"
	"strings"
)

// mcqVerdict scores a multiple-choice answer (spec 0014 decision C, ADR
// 0013): extract the single option letter the model committed to and compare
// it exactly with the case's expected letter. Extraction is conservative by
// design — no letter or conflicting letters means a miss (0), never a guess.
// A miss is still a scored verdict (the model did answer), so the score is
// never nil; only an answer-call failure keeps the case unscored (W7).
func mcqVerdict(expected, answer string) (*float64, string) {
	want := strings.ToUpper(strings.TrimSpace(expected))
	letter, found := extractOptionLetter(answer)
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

// extractOptionLetter pulls the single option letter (A-D) a model committed
// to out of its answer. The answer is first folded through the v2
// normalization pipeline (NFKC maps full-width letters onto ASCII), then
// every pattern's captures are collected case-insensitively. Exactly one
// distinct letter across all captures is an extraction; zero or conflicting
// letters are not — conservative beats clever, a miss is preferable to a
// guessed hit.
func extractOptionLetter(answer string) (string, bool) {
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
	if len(found) != 1 {
		return "", false
	}
	for letter := range found {
		return string(letter), true
	}
	return "", false
}
