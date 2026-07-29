package ifeval

import (
	"regexp"
	"strings"
	"unicode"
)

// textutil.go ports the deterministic text utilities of the official
// instructions_util.py that the checkers rely on. Regex patterns are copied
// verbatim from the official source (Python re and Go RE2 agree on every
// construct used here; Python's \s/\w are Unicode-aware while RE2's \s is
// ASCII-only — immaterial for these patterns on IFEval English responses,
// noted where relevant).

// splitIntoSentences ports instructions_util.split_into_sentences verbatim
// (the official deterministic regex sentence splitter). The official
// count_sentences uses the nltk Punkt model instead; see the package doc for
// the sanctioned approximation. Replacement-order fidelity matters: every
// substitution below runs in the official sequence.
func splitIntoSentences(text string) []string {
	text = " " + text + "  "
	text = strings.ReplaceAll(text, "\n", " ")
	text = rePrefixes.ReplaceAllString(text, "${1}<prd>")
	text = reWebsites.ReplaceAllString(text, "<prd>${1}")
	text = reDigitDotDigit.ReplaceAllString(text, "${1}<prd>${2}")
	text = reMultipleDots.ReplaceAllStringFunc(text, func(m string) string {
		return strings.Repeat("<prd>", len(m)) + "<stop>"
	})
	if strings.Contains(text, "Ph.D") {
		text = strings.ReplaceAll(text, "Ph.D.", "Ph<prd>D<prd>")
	}
	text = reSingleLetterDot.ReplaceAllString(text, " ${1}<prd> ")
	text = reAcronymStarter.ReplaceAllString(text, "${1}<stop> ${2}")
	text = reThreeLetters.ReplaceAllString(text, "${1}<prd>${2}<prd>${3}<prd>")
	text = reTwoLetters.ReplaceAllString(text, "${1}<prd>${2}<prd>")
	text = reSuffixStarter.ReplaceAllString(text, " ${1}<stop> ${2}")
	text = reSuffixDot.ReplaceAllString(text, " ${1}<prd>")
	text = reAlphaDot.ReplaceAllString(text, " ${1}<prd>")
	if strings.Contains(text, "”") {
		text = strings.ReplaceAll(text, ".”", "”.")
	}
	if strings.Contains(text, "\"") {
		text = strings.ReplaceAll(text, ".\"", "\".")
	}
	if strings.Contains(text, "!") {
		text = strings.ReplaceAll(text, "!\"", "\"!")
	}
	if strings.Contains(text, "?") {
		text = strings.ReplaceAll(text, "?\"", "\"?")
	}
	text = strings.ReplaceAll(text, ".", ".<stop>")
	text = strings.ReplaceAll(text, "?", "?<stop>")
	text = strings.ReplaceAll(text, "!", "!<stop>")
	text = strings.ReplaceAll(text, "<prd>", ".")
	parts := strings.Split(text, "<stop>")
	sentences := make([]string, 0, len(parts))
	for _, p := range parts {
		sentences = append(sentences, strings.TrimSpace(p))
	}
	if len(sentences) > 0 && sentences[len(sentences)-1] == "" {
		sentences = sentences[:len(sentences)-1]
	}
	return sentences
}

// Official patterns from instructions_util.py (lines 62-69), verbatim.
var (
	rePrefixes        = regexp.MustCompile(`(Mr|St|Mrs|Ms|Dr)[.]`)
	reWebsites        = regexp.MustCompile(`[.](com|net|org|io|gov|edu|me)`)
	reDigitDotDigit   = regexp.MustCompile(`([0-9])[.]([0-9])`)
	reMultipleDots    = regexp.MustCompile(`\.{2,}`)
	reSingleLetterDot = regexp.MustCompile(`\s([A-Za-z])[.] `)
	reAcronymStarter  = regexp.MustCompile(`([A-Z][.][A-Z][.](?:[A-Z][.])?) (` + sentenceStarters + `)`)
	reThreeLetters    = regexp.MustCompile(`([A-Za-z])[.]([A-Za-z])[.]([A-Za-z])[.]`)
	reTwoLetters      = regexp.MustCompile(`([A-Za-z])[.]([A-Za-z])[.]`)
	reSuffixStarter   = regexp.MustCompile(` (Inc|Ltd|Jr|Sr|Co)[.] (` + sentenceStarters + `)`)
	reSuffixDot       = regexp.MustCompile(` (Inc|Ltd|Jr|Sr|Co)[.]`)
	reAlphaDot        = regexp.MustCompile(` ([A-Za-z])[.]`)
)

// sentenceStarters is the official _STARTERS alternation (the trailing
// starters have no \s suffix upstream).
const sentenceStarters = `Mr|Mrs|Ms|Dr|Prof|Capt|Cpt|Lt|He\s|She\s|It\s|They\s|Their\s|Our\s|We\s|But\s|However\s|That\s|This\s|Wherever`

// countSentences is the sanctioned port of instructions_util.count_sentences
// (package doc, approximation 1): the official uses the nltk Punkt
// tokenizer; the port counts the official regex splitter's output.
func countSentences(text string) int {
	return len(splitIntoSentences(text))
}

// reWordToken approximates nltk RegexpTokenizer(r"\w+") used by
// instructions_util.count_words: Python's \w is Unicode-aware, so the port
// uses letters/digits/underscore across scripts (Python \w additionally
// admits some combining marks — an accepted minor deviation).
var reWordToken = regexp.MustCompile(`[\p{L}\p{N}_]+`)

// countWords ports instructions_util.count_words.
func countWords(text string) int {
	return len(reWordToken.FindAllString(text, -1))
}

// isUpperPython ports Python str.isupper: at least one cased rune, and every
// cased rune uppercase.
func isUpperPython(s string) bool {
	cased := false
	for _, r := range s {
		if unicode.IsLower(r) {
			return false
		}
		if unicode.IsUpper(r) {
			cased = true
		}
	}
	return cased
}
