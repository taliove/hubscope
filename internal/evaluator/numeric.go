package evaluator

import (
	"fmt"
	"regexp"
	"strings"
)

// numericVerdict scores a numeric-answer response (ticket 95, spec 0014
// decision C, ADR 0013): extract the final number the model committed to and
// compare it exactly with the case's expected value after normalization.
// Extraction is conservative by design — no extractable number means a miss
// (0), never a guess. A miss is still a scored verdict (the model did
// answer), so the score is never nil; only an answer-call failure keeps the
// case unscored (W7).
func numericVerdict(expected, answer string) (*float64, string) {
	want := normalizeNumericToken(expected)
	got, found := extractFinalNumber(answer)
	if !found {
		return scorePtr(0), fmt.Sprintf("rule numeric: no numeric answer extracted (expected %q)", expected)
	}
	if got == want {
		return scorePtr(1), fmt.Sprintf("rule numeric matched (expected %q)", expected)
	}
	return scorePtr(0), fmt.Sprintf("rule numeric answered %q (expected %q)", got, expected)
}

// numberToken matches one numeric literal: optional sign, integer part with
// optional comma thousands separators, optional decimal part. A leading "$"
// or trailing "%"/unit is deliberately not part of the token — the number
// itself is the answer.
var numberToken = regexp.MustCompile(`-?\d[\d,]*(?:\.\d+)?`)

// extractFinalNumber pulls the final numeric answer out of a model reply,
// following the official GSM8K evaluation convention:
//
//  1. The first number after the LAST "####" marker wins (the official
//     answer line).
//  2. Otherwise the last number of the last non-empty line wins.
//
// Anything else is not an extraction — conservative beats clever, a miss is
// preferable to a guessed hit. Each line is folded through the v2
// normalization pipeline separately (NFKC maps full-width digits onto
// ASCII); normalizing per line preserves the line structure the rule is
// defined on, which the whole-text v2 pipeline would collapse.
func extractFinalNumber(answer string) (string, bool) {
	lines := strings.Split(answer, "\n")
	for i := range lines {
		lines[i] = normalizeV2(lines[i])
	}
	for i := len(lines) - 1; i >= 0; i-- {
		if idx := strings.LastIndex(lines[i], "####"); idx >= 0 {
			if m := numberToken.FindString(lines[i][idx+4:]); m != "" {
				return normalizeNumericToken(m), true
			}
		}
	}
	for i := len(lines) - 1; i >= 0; i-- {
		if lines[i] == "" {
			continue
		}
		all := numberToken.FindAllString(lines[i], -1)
		if len(all) == 0 {
			return "", false
		}
		return normalizeNumericToken(all[len(all)-1]), true
	}
	return "", false
}

// normalizeNumericToken canonicalizes a numeric literal so equivalent
// spellings compare equal: thousands separators removed, leading "+"
// dropped, decimal trailing zeros trimmed ("42.0" -> "42"), and "-0" folded
// to "0". The expected side passes through the same canonicalization, so
// the comparison stays symmetric.
func normalizeNumericToken(tok string) string {
	s := strings.TrimSpace(tok)
	s = strings.TrimPrefix(s, "$")
	s = strings.ReplaceAll(s, ",", "")
	s = strings.TrimPrefix(s, "+")
	if strings.Contains(s, ".") {
		s = strings.TrimRight(s, "0")
		s = strings.TrimRight(s, ".")
	}
	if s == "-0" || s == "" {
		s = "0"
	}
	return s
}
