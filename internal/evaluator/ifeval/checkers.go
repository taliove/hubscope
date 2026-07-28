package ifeval

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

// checkers.go holds the 22 ported instruction checkers, one per official
// class in instruction_following_eval/instructions.py (master, fetched
// 2026-07-28). Every checker cites its official class; regex patterns are
// copied verbatim from the official source. Validators mirror the kwarg
// validations inside the official build_description methods, rejecting
// (fail-closed) the inputs for which the official code would fall back to a
// RANDOM parameter — a randomized check is unmeasurable and must never
// enter the frozen bank.

// registry maps each ported instruction id to its spec. The three
// langdetect-dependent official types are deliberately absent (package doc).
var registry = map[string]instructionSpec{
	"keywords:existence": {
		validate: validateKeywordsExistence,
		check:    checkKeywordsExistence,
	},
	"keywords:frequency": {
		validate: validateKeywordFrequency,
		check:    checkKeywordFrequency,
	},
	"keywords:forbidden_words": {
		validate: validateForbiddenWords,
		check:    checkForbiddenWords,
	},
	"keywords:letter_frequency": {
		validate: validateLetterFrequency,
		check:    checkLetterFrequency,
	},
	"length_constraints:number_sentences": {
		validate: validateNumberSentences,
		check:    checkNumberSentences,
	},
	"length_constraints:number_paragraphs": {
		validate: validateNumberParagraphs,
		check:    checkNumberParagraphs,
	},
	"length_constraints:number_words": {
		validate: validateNumberWords,
		check:    checkNumberWords,
	},
	"length_constraints:nth_paragraph_first_word": {
		validate: validateNthParagraphFirstWord,
		check:    checkNthParagraphFirstWord,
	},
	"detectable_content:number_placeholders": {
		validate: validateNumberPlaceholders,
		check:    checkNumberPlaceholders,
	},
	"detectable_content:postscript": {
		validate: validatePostscript,
		check:    checkPostscript,
	},
	"detectable_format:number_bullet_lists": {
		validate: validateNumberBulletLists,
		check:    checkNumberBulletLists,
	},
	"detectable_format:constrained_response": {
		validate: validateNoKwargs,
		check:    checkConstrainedResponse,
	},
	"detectable_format:number_highlighted_sections": {
		validate: validateNumberHighlights,
		check:    checkNumberHighlights,
	},
	"detectable_format:multiple_sections": {
		validate: validateMultipleSections,
		check:    checkMultipleSections,
	},
	"detectable_format:json_format": {
		validate: validateNoKwargs,
		check:    checkJSONFormat,
	},
	"detectable_format:title": {
		validate: validateNoKwargs,
		check:    checkTitle,
	},
	"combination:two_responses": {
		validate: validateNoKwargs,
		check:    checkTwoResponses,
	},
	"combination:repeat_prompt": {
		validate: validateRepeatPrompt,
		check:    checkRepeatPrompt,
	},
	"startend:end_checker": {
		validate: validateEndChecker,
		check:    checkEndChecker,
	},
	"change_case:capital_word_frequency": {
		validate: validateCapitalWordFrequency,
		check:    checkCapitalWordFrequency,
	},
	"punctuation:no_comma": {
		validate: validateNoKwargs,
		check:    checkNoComma,
	},
	"startend:quotation": {
		validate: validateNoKwargs,
		check:    checkQuotation,
	},
}

// compileCaseInsensitive compiles a pattern with Python re.IGNORECASE
// semantics; a keyword that cannot compile is a data bug, caught at
// validation time.
func compileCaseInsensitive(pattern string) (*regexp.Regexp, error) {
	return regexp.Compile("(?i)" + pattern)
}

// --- keywords:existence — official KeywordChecker (instructions.py:703) ---
// check_following: re.search(keyword, value, IGNORECASE) for every keyword
// (the keyword itself is the regex pattern, verbatim).

func validateKeywordsExistence(kw kwargs) error {
	keywords, ok := kw.strList("keywords")
	if !ok || len(keywords) == 0 {
		return fmt.Errorf("kwargs.keywords must be a non-empty string list")
	}
	for _, keyword := range keywords {
		if _, err := compileCaseInsensitive(keyword); err != nil {
			return fmt.Errorf("kwargs.keywords entry %q is not a compilable pattern: %v", keyword, err)
		}
	}
	return nil
}

func checkKeywordsExistence(kw kwargs, response string) (bool, error) {
	keywords, _ := kw.strList("keywords")
	for _, keyword := range keywords {
		re, err := compileCaseInsensitive(keyword)
		if err != nil {
			return false, err
		}
		if !re.MatchString(response) {
			return false, nil
		}
	}
	return true, nil
}

// --- keywords:frequency — official KeywordFrequencyChecker (:745) --------
// check_following: len(re.findall(keyword.strip(), value, IGNORECASE))
// compared by relation.

func validateKeywordFrequency(kw kwargs) error {
	keyword, ok := kw.str("keyword")
	if !ok || strings.TrimSpace(keyword) == "" {
		return fmt.Errorf("kwargs.keyword must be a non-empty string")
	}
	if err := requireNonNegInt(kw, "frequency"); err != nil {
		return err
	}
	if err := requireRelation(kw, "relation"); err != nil {
		return err
	}
	if _, err := compileCaseInsensitive(strings.TrimSpace(keyword)); err != nil {
		return fmt.Errorf("kwargs.keyword %q is not a compilable pattern: %v", keyword, err)
	}
	return nil
}

func checkKeywordFrequency(kw kwargs, response string) (bool, error) {
	keyword, _ := kw.str("keyword")
	frequency, _ := kw.num("frequency")
	relation, _ := kw.str("relation")
	re, err := compileCaseInsensitive(strings.TrimSpace(keyword))
	if err != nil {
		return false, err
	}
	actual := len(re.FindAllString(response, -1))
	return compareRelation(relation, actual, frequency)
}

// --- keywords:forbidden_words — official ForbiddenWords (:1070) ----------
// check_following: none of the words matches \b<word>\b case-insensitively
// (RE2 \b is ASCII; dataset words are ASCII — noted deviation).

func validateForbiddenWords(kw kwargs) error {
	words, ok := kw.strList("forbidden_words")
	if !ok || len(words) == 0 {
		return fmt.Errorf("kwargs.forbidden_words must be a non-empty string list")
	}
	for _, word := range words {
		if _, err := compileCaseInsensitive(`\b` + word + `\b`); err != nil {
			return fmt.Errorf("kwargs.forbidden_words entry %q is not a compilable pattern: %v", word, err)
		}
	}
	return nil
}

func checkForbiddenWords(kw kwargs, response string) (bool, error) {
	words, _ := kw.strList("forbidden_words")
	for _, word := range words {
		re, err := compileCaseInsensitive(`\b` + word + `\b`)
		if err != nil {
			return false, err
		}
		if re.MatchString(response) {
			return false, nil
		}
	}
	return true, nil
}

// --- keywords:letter_frequency — official LetterFrequencyChecker (:1316) --
// check_following: count of the (lowercased) letter in the lowercased
// response, compared by relation.

func validateLetterFrequency(kw kwargs) error {
	letter, ok := kw.str("letter")
	if !ok {
		return fmt.Errorf("kwargs.letter must be a string")
	}
	letter = strings.ToLower(strings.TrimSpace(letter))
	if len(letter) != 1 || letter[0] < 'a' || letter[0] > 'z' {
		// The official build_description RANDOMIZES the letter for out-of-range
		// input; the port rejects it fail-closed instead.
		return fmt.Errorf("kwargs.letter must be a single letter a-z, got %q", letter)
	}
	if err := requireNonNegInt(kw, "let_frequency"); err != nil {
		return err
	}
	return requireRelation(kw, "let_relation")
}

func checkLetterFrequency(kw kwargs, response string) (bool, error) {
	letter, _ := kw.str("letter")
	letter = strings.ToLower(strings.TrimSpace(letter))
	frequency, _ := kw.num("let_frequency")
	relation, _ := kw.str("let_relation")
	actual := strings.Count(strings.ToLower(response), letter)
	return compareRelation(relation, actual, frequency)
}

// --- length_constraints:number_sentences — official NumberOfSentences ----
// (:167). check_following: count_sentences(value) compared by relation. See
// the package doc for the sanctioned splitter approximation.

func validateNumberSentences(kw kwargs) error {
	if err := requireNonNegInt(kw, "num_sentences"); err != nil {
		return err
	}
	return requireRelation(kw, "relation")
}

func checkNumberSentences(kw kwargs, response string) (bool, error) {
	threshold, _ := kw.num("num_sentences")
	relation, _ := kw.str("relation")
	return compareRelation(relation, countSentences(response), threshold)
}

// --- length_constraints:number_paragraphs — official ParagraphChecker ----
// (:530). check_following: split on the markdown divider `\s?\*\*\*\s?`;
// empty edge paragraphs are discounted, an empty middle paragraph fails, and
// the resulting count must equal num_paragraphs exactly.

var reParagraphDivider = regexp.MustCompile(`\s?\*\*\*\s?`)

func validateNumberParagraphs(kw kwargs) error {
	return requireNonNegInt(kw, "num_paragraphs")
}

func checkNumberParagraphs(kw kwargs, response string) (bool, error) {
	expected, _ := kw.num("num_paragraphs")
	paragraphs := reParagraphDivider.Split(response, -1)
	num := len(paragraphs)
	for i, p := range paragraphs {
		if strings.TrimSpace(p) == "" {
			if i == 0 || i == len(paragraphs)-1 {
				num--
			} else {
				return false, nil
			}
		}
	}
	return num == expected, nil
}

// --- length_constraints:number_words — official NumberOfWords (:813) -----
// check_following: count_words(value) compared by relation.

func validateNumberWords(kw kwargs) error {
	if err := requireNonNegInt(kw, "num_words"); err != nil {
		return err
	}
	return requireRelation(kw, "relation")
}

func checkNumberWords(kw kwargs, response string) (bool, error) {
	threshold, _ := kw.num("num_words")
	relation, _ := kw.str("relation")
	return compareRelation(relation, countWords(response), threshold)
}

// --- length_constraints:nth_paragraph_first_word — official ---------------
// ParagraphFirstWordCheck (:908). check_following: split on "\n\n"; the
// paragraph count discounts blank paragraphs but the nth-paragraph lookup
// indexes the RAW split (official subtlety, ported verbatim); the first word
// is taken up to the first of .,?!'" and compared lowercased.

func validateNthParagraphFirstWord(kw kwargs) error {
	numParagraphs, ok := kw.num("num_paragraphs")
	if !ok || numParagraphs < 0 {
		return fmt.Errorf("kwargs.num_paragraphs must be an integer >= 0")
	}
	nth, ok := kw.num("nth_paragraph")
	if !ok || nth <= 0 || nth > numParagraphs {
		// The official build_description RANDOMIZES nth_paragraph when it is
		// out of range; the port rejects fail-closed.
		return fmt.Errorf("kwargs.nth_paragraph must be within 1..num_paragraphs")
	}
	firstWord, ok := kw.str("first_word")
	if !ok || firstWord == "" {
		return fmt.Errorf("kwargs.first_word must be a non-empty string")
	}
	return nil
}

func checkNthParagraphFirstWord(kw kwargs, response string) (bool, error) {
	numParagraphs, _ := kw.num("num_paragraphs")
	nth, _ := kw.num("nth_paragraph")
	wantWord, _ := kw.str("first_word")
	wantWord = strings.ToLower(wantWord)

	paragraphs := strings.Split(response, "\n\n")
	num := len(paragraphs)
	for _, p := range paragraphs {
		if strings.TrimSpace(p) == "" {
			num--
		}
	}
	if nth > num {
		return false, nil
	}
	paragraph := strings.TrimSpace(paragraphs[nth-1])
	if paragraph == "" {
		return false, nil
	}

	// Official first-word extraction (:993-1005): first whitespace token,
	// leading ' and " stripped, then letters up to the first punctuation of
	// .,?!'", lowercased.
	word := strings.TrimSpace(strings.Fields(paragraph)[0])
	word = strings.TrimLeft(word, "'")
	word = strings.TrimLeft(word, "\"")
	punctuation := map[rune]bool{'.': true, ',': true, '?': true, '!': true, '\'': true, '"': true}
	var firstWord strings.Builder
	for _, r := range word {
		if punctuation[r] {
			break
		}
		firstWord.WriteRune(unicode.ToLower(r))
	}
	return num == numParagraphs && firstWord.String() == wantWord, nil
}

// --- detectable_content:number_placeholders — official PlaceholderChecker -
// (:235). check_following: len(re.findall(r"\[.*?\]", value)) >= num.

var rePlaceholder = regexp.MustCompile(`\[.*?\]`)

func validateNumberPlaceholders(kw kwargs) error {
	return requireNonNegInt(kw, "num_placeholders")
}

func checkNumberPlaceholders(kw kwargs, response string) (bool, error) {
	expected, _ := kw.num("num_placeholders")
	return len(rePlaceholder.FindAllString(response, -1)) >= expected, nil
}

// --- detectable_content:postscript — official PostscriptChecker (:584) ----
// check_following: the lowercased response is searched (MULTILINE) for a
// postscript pattern; "P.P.S" and "P.S." get special regexes, any other
// marker is interpolated raw (dataset only carries these two).

func validatePostscript(kw kwargs) error {
	marker, ok := kw.str("postscript_marker")
	if !ok || marker == "" {
		return fmt.Errorf("kwargs.postscript_marker must be a non-empty string")
	}
	if _, err := postscriptPattern(marker); err != nil {
		return err
	}
	return nil
}

// postscriptPattern mirrors the official pattern selection (:629-634).
func postscriptPattern(marker string) (*regexp.Regexp, error) {
	var pattern string
	switch marker {
	case "P.P.S":
		pattern = `\s*p\.\s?p\.\s?s.*$`
	case "P.S.":
		pattern = `\s*p\.\s?s\..*$`
	default:
		pattern = `\s*` + strings.ToLower(marker) + `.*$`
	}
	return regexp.Compile("(?m)" + pattern)
}

func checkPostscript(kw kwargs, response string) (bool, error) {
	marker, _ := kw.str("postscript_marker")
	re, err := postscriptPattern(marker)
	if err != nil {
		return false, err
	}
	return re.MatchString(strings.ToLower(response)), nil
}

// --- detectable_format:number_bullet_lists — official BulletListChecker ---
// (:280). check_following: lines starting with `*` (not `**`) or `-`,
// counted together, must equal num_bullets exactly.

var (
	reBulletStar = regexp.MustCompile(`(?m)^\s*\*[^\*].*$`)
	reBulletDash = regexp.MustCompile(`(?m)^\s*-.*$`)
)

func validateNumberBulletLists(kw kwargs) error {
	return requireNonNegInt(kw, "num_bullets")
}

func checkNumberBulletLists(kw kwargs, response string) (bool, error) {
	expected, _ := kw.num("num_bullets")
	actual := len(reBulletStar.FindAllString(response, -1)) + len(reBulletDash.FindAllString(response, -1))
	return actual == expected, nil
}

// --- detectable_format:constrained_response — official --------------------
// ConstrainedResponseChecker (:329). check_following: the stripped response
// contains one of the fixed options (_CONSTRAINED_RESPONSE_OPTIONS).

var constrainedResponseOptions = []string{
	"My answer is yes.", "My answer is no.", "My answer is maybe.",
}

func checkConstrainedResponse(kw kwargs, response string) (bool, error) {
	trimmed := strings.TrimSpace(response)
	for _, option := range constrainedResponseOptions {
		if strings.Contains(trimmed, option) {
			return true, nil
		}
	}
	return false, nil
}

// --- detectable_format:number_highlighted_sections — official -------------
// HighlightSectionChecker (:411). check_following: single-star and
// double-star markdown highlights with non-empty content, counted together,
// must be >= num_highlights.

var (
	reHighlightSingle = regexp.MustCompile(`\*[^\n\*]*\*`)
	reHighlightDouble = regexp.MustCompile(`\*\*[^\n\*]*\*\*`)
)

func validateNumberHighlights(kw kwargs) error {
	return requireNonNegInt(kw, "num_highlights")
}

func checkNumberHighlights(kw kwargs, response string) (bool, error) {
	expected, _ := kw.num("num_highlights")
	num := 0
	for _, h := range reHighlightSingle.FindAllString(response, -1) {
		if strings.TrimSpace(strings.Trim(h, "*")) != "" {
			num++
		}
	}
	for _, h := range reHighlightDouble.FindAllString(response, -1) {
		content := strings.TrimSuffix(strings.TrimPrefix(h, "**"), "**")
		if strings.TrimSpace(content) != "" {
			num++
		}
	}
	return num >= expected, nil
}

// --- detectable_format:multiple_sections — official SectionChecker --------
// (:466). check_following: split on `\s?<spliter>\s?\d+\s?` (the spliter is
// interpolated raw), number of sections = splits - 1, must be >= num.

func validateMultipleSections(kw kwargs) error {
	spliter, ok := kw.str("section_spliter")
	if !ok || strings.TrimSpace(spliter) == "" {
		return fmt.Errorf("kwargs.section_spliter must be a non-empty string")
	}
	if _, err := sectionSplitPattern(spliter); err != nil {
		return fmt.Errorf("kwargs.section_spliter %q is not a compilable pattern: %v", spliter, err)
	}
	return requireNonNegInt(kw, "num_sections")
}

// sectionSplitPattern mirrors the official splitter pattern (:524). The
// official strips the spliter before use.
func sectionSplitPattern(spliter string) (*regexp.Regexp, error) {
	return regexp.Compile(`\s?` + strings.TrimSpace(spliter) + `\s?\d+\s?`)
}

func checkMultipleSections(kw kwargs, response string) (bool, error) {
	spliter, _ := kw.str("section_spliter")
	expected, _ := kw.num("num_sections")
	re, err := sectionSplitPattern(spliter)
	if err != nil {
		return false, err
	}
	num := len(re.Split(response, -1)) - 1
	return num >= expected, nil
}

// --- detectable_format:json_format — official JsonFormat (:873) -----------
// check_following: strip markdown fences (four prefixes, one suffix, in the
// official order) then json.loads. Go's encoding/json rejects Python's
// NaN/Infinity literals — an accepted minor deviation.

func checkJSONFormat(kw kwargs, response string) (bool, error) {
	v := strings.TrimSpace(response)
	v = strings.TrimPrefix(v, "```json")
	v = strings.TrimPrefix(v, "```Json")
	v = strings.TrimPrefix(v, "```JSON")
	v = strings.TrimPrefix(v, "```")
	v = strings.TrimSuffix(v, "```")
	v = strings.TrimSpace(v)
	var js any
	return json.Unmarshal([]byte(v), &js) == nil, nil
}

// --- detectable_format:title — official TitleChecker (:1286) --------------
// check_following: a <<title>> with non-empty content exists.

var reTitle = regexp.MustCompile(`<<[^\n]+>>`)

func checkTitle(kw kwargs, response string) (bool, error) {
	for _, title := range reTitle.FindAllString(response, -1) {
		content := strings.TrimRight(strings.TrimLeft(title, "<"), ">")
		if strings.TrimSpace(content) != "" {
			return true, nil
		}
	}
	return false, nil
}

// --- combination:two_responses — official TwoResponsesChecker (:1171) -----
// check_following: split on "******"; blank chunks are only tolerated at the
// edges; exactly two distinct non-blank responses must remain.

func checkTwoResponses(kw kwargs, response string) (bool, error) {
	responses := strings.Split(response, "******")
	var valid []string
	for i, r := range responses {
		if strings.TrimSpace(r) == "" {
			if i != 0 && i != len(responses)-1 {
				return false, nil
			}
		} else {
			valid = append(valid, r)
		}
	}
	return len(valid) == 2 && strings.TrimSpace(valid[0]) != strings.TrimSpace(valid[1]), nil
}

// --- combination:repeat_prompt — official RepeatPromptThenAnswer (:1213) --
// check_following: the response starts with the prompt, both trimmed and
// lowercased.

func validateRepeatPrompt(kw kwargs) error {
	prompt, ok := kw.str("prompt_to_repeat")
	if !ok || prompt == "" {
		return fmt.Errorf("kwargs.prompt_to_repeat must be a non-empty string")
	}
	return nil
}

func checkRepeatPrompt(kw kwargs, response string) (bool, error) {
	prompt, _ := kw.str("prompt_to_repeat")
	return strings.HasPrefix(
		strings.ToLower(strings.TrimSpace(response)),
		strings.ToLower(strings.TrimSpace(prompt)),
	), nil
}

// --- startend:end_checker — official EndChecker (:1250) -------------------
// check_following: response stripped, surrounding double quotes stripped,
// lowercased; must end with the stripped lowercased end phrase.

func validateEndChecker(kw kwargs) error {
	phrase, ok := kw.str("end_phrase")
	if !ok || strings.TrimSpace(phrase) == "" {
		return fmt.Errorf("kwargs.end_phrase must be a non-empty string")
	}
	return nil
}

func checkEndChecker(kw kwargs, response string) (bool, error) {
	phrase, _ := kw.str("end_phrase")
	v := strings.ToLower(strings.Trim(strings.TrimSpace(response), "\""))
	return strings.HasSuffix(v, strings.ToLower(strings.TrimSpace(phrase))), nil
}

// --- change_case:capital_word_frequency — official ------------------------
// CapitalWordFrequencyChecker (:1479). check_following: words tokenized with
// nltk.word_tokenize, those with str.isupper() counted, compared by
// relation. See the package doc for the sanctioned tokenizer approximation
// (whitespace fields; isupper ported exactly).

func validateCapitalWordFrequency(kw kwargs) error {
	if _, ok := kw.num("capital_frequency"); !ok {
		return fmt.Errorf("kwargs.capital_frequency must be an integer")
	}
	return requireRelation(kw, "capital_relation")
}

func checkCapitalWordFrequency(kw kwargs, response string) (bool, error) {
	frequency, _ := kw.num("capital_frequency")
	relation, _ := kw.str("capital_relation")
	num := 0
	for _, word := range strings.Fields(response) {
		if isUpperPython(word) {
			num++
		}
	}
	return compareRelation(relation, num, frequency)
}

// --- punctuation:no_comma — official CommaChecker (:1457) -----------------
// check_following: no comma anywhere in the response.

func checkNoComma(kw kwargs, response string) (bool, error) {
	return !strings.Contains(response, ","), nil
}

// --- startend:quotation — official QuotationChecker (:1545) ---------------
// check_following: the stripped response is longer than one char and wrapped
// in double quotes.

func checkQuotation(kw kwargs, response string) (bool, error) {
	v := strings.TrimSpace(response)
	return len(v) > 1 && v[0] == '"' && v[len(v)-1] == '"', nil
}

// --- shared validators -----------------------------------------------------

// validateNoKwargs accepts kwarg-less instruction types (the official
// build_description methods take no parameters; stray keys are ignored
// upstream and here).
func validateNoKwargs(kw kwargs) error {
	return nil
}
