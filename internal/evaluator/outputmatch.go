package evaluator

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// outputMatchVerdict scores a CRUXEval-style output prediction (ticket 98,
// spec 0014 decision C, ADR 0013): the model predicts the output of f(input)
// as a Python literal, and the prediction is compared against the standard
// answer that was precomputed offline (scripts/cruxeval_subset.py) — the
// runtime never executes any code, this is pure literal comparison.
//
// Normalization profile (ADR 0008 family): surrounding whitespace/newlines
// are insignificant, quote style ('a' vs "a") and container spacing
// ([1, 2] vs [1,2]) normalize away, True/False/None match case-insensitively,
// and dict/set element order is insignificant (Python == semantics). The
// caliber is conservative: if no unambiguous literal can be extracted from
// the answer, or the answer does not parse as a Python literal, the case
// scores 0 — never a guessed hit. A miss is still a scored verdict (the
// model did answer); only an answer-call failure keeps the case unscored (W7).
func outputMatchVerdict(expected, answer string) (*float64, string) {
	want, ok := CanonicalPyLiteral(expected)
	if !ok {
		// Seed/admin data bug, not a model miss: report a rule error and
		// leave the case unscored rather than inventing a caliber.
		return nil, fmt.Sprintf("rule error: output_match expected %q is not a Python literal", expected)
	}
	literal, found := extractOutputLiteral(answer)
	if !found {
		return scorePtr(0), fmt.Sprintf("rule output_match: no unambiguous literal extracted (expected %q)", expected)
	}
	got, ok := CanonicalPyLiteral(literal)
	if !ok {
		return scorePtr(0), fmt.Sprintf("rule output_match: answer is not a Python literal (expected %q)", expected)
	}
	if got == want {
		return scorePtr(1), fmt.Sprintf("rule output_match matched (expected %q)", expected)
	}
	return scorePtr(0), fmt.Sprintf("rule output_match answered %q (expected %q)", truncateRunes(literal, 80), expected)
}

// extractOutputLiteral pulls the single output literal the model committed to
// out of its reply. The cast prompt demands the bare literal; the unwraps
// below cover the official CRUXEval answer tags and the shapes chat models
// actually emit. Anything ambiguous (multiple fences, prose around a fence)
// fails extraction — conservative beats clever.
func extractOutputLiteral(answer string) (string, bool) {
	s := strings.TrimSpace(answer)
	if s == "" {
		return "", false
	}
	// Official CRUXEval caliber: the answer inside [ANSWER] [/ANSWER] tags.
	if idx := strings.Index(s, "[ANSWER]"); idx >= 0 {
		rest := s[idx+len("[ANSWER]"):]
		if end := strings.Index(rest, "[/ANSWER]"); end >= 0 {
			rest = rest[:end]
		}
		s = strings.TrimSpace(rest)
	}
	// A single code fence wrapping exactly the literal.
	if strings.HasPrefix(s, "```") {
		end := strings.Index(s[3:], "```")
		if end < 0 {
			return "", false
		}
		inner := s[3 : 3+end]
		if outside := strings.TrimSpace(s[3+end+3:]); outside != "" {
			return "", false
		}
		// Strip an optional language tag on the fence's first line.
		if nl := strings.IndexByte(inner, '\n'); nl >= 0 {
			if tag := strings.TrimSpace(inner[:nl]); tag != "" && !strings.ContainsAny(tag, " \t") {
				inner = inner[nl+1:]
			}
		}
		s = strings.TrimSpace(inner)
	}
	// Assertion form ("assert f(x) == 3", "f(x) == 3", "== 3"): keep what
	// follows the first top-level ==, mirroring the official postprocessing.
	if idx := topLevelDoubleEq(s); idx >= 0 {
		s = strings.TrimSpace(s[idx+2:])
	}
	if s == "" {
		return "", false
	}
	return s, true
}

// topLevelDoubleEq returns the byte index of the first "==" occurring outside
// string quotes, or -1.
func topLevelDoubleEq(s string) int {
	var quote byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		if quote != 0 {
			if c == '\\' {
				i++
			} else if c == quote {
				quote = 0
			}
			continue
		}
		if c == '\'' || c == '"' {
			quote = c
			continue
		}
		if c == '=' && i+1 < len(s) && s[i+1] == '=' {
			return i
		}
	}
	return -1
}

// CanonicalPyLiteral parses s as a Python literal (string, bytes, number,
// True/False/None, list, tuple, dict, set, nested) and returns its canonical
// form: whitespace outside strings removed, strings re-quoted with double
// quotes, True/False/None accepted case-insensitively, dict entries and set
// elements sorted (Python == is order-insensitive for both). ok is false when
// s is not exactly one literal — unsimplified expressions, calls, and
// trailing garbage all fail.
func CanonicalPyLiteral(s string) (string, bool) {
	p := &pyParser{s: strings.TrimSpace(s)}
	v, ok := p.parseValue()
	if !ok {
		return "", false
	}
	p.skipWS()
	if p.i != len(p.s) {
		return "", false
	}
	return v, true
}

// pyParser is a recursive-descent parser over a Python literal. It operates
// on bytes; multi-byte UTF-8 inside strings passes through untouched.
type pyParser struct {
	s string
	i int
}

func (p *pyParser) skipWS() {
	for p.i < len(p.s) {
		switch p.s[p.i] {
		case ' ', '\t', '\n', '\r', '\f', '\v':
			p.i++
		default:
			return
		}
	}
}

func (p *pyParser) parseValue() (string, bool) {
	p.skipWS()
	if p.i >= len(p.s) {
		return "", false
	}
	c := p.s[p.i]
	switch {
	case c == '\'' || c == '"':
		return p.parseString()
	case (c == 'b' || c == 'B') && p.i+1 < len(p.s) && (p.s[p.i+1] == '\'' || p.s[p.i+1] == '"'):
		return p.parseString()
	case c == '[':
		return p.parseList()
	case c == '(':
		return p.parseTuple()
	case c == '{':
		return p.parseDictOrSet()
	case c == '-' || c == '+' || c == '.' || (c >= '0' && c <= '9'):
		return p.parseNumber()
	case c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z':
		return p.parseKeyword()
	}
	return "", false
}

// parseString parses a quoted string or bytes literal and returns its
// canonical double-quoted form (b-prefixed for bytes). Only the escape
// sequences repr() actually emits are understood; anything else fails
// conservatively.
func (p *pyParser) parseString() (string, bool) {
	isBytes := false
	if p.s[p.i] == 'b' || p.s[p.i] == 'B' {
		isBytes = true
		p.i++
	}
	q := p.s[p.i]
	p.i++
	var b strings.Builder
	for p.i < len(p.s) {
		c := p.s[p.i]
		if c == q {
			p.i++
			prefix := ""
			if isBytes {
				prefix = "b"
			}
			return prefix + `"` + escapePyString(b.String()) + `"`, true
		}
		if c == '\\' {
			p.i++
			if p.i >= len(p.s) {
				return "", false
			}
			switch e := p.s[p.i]; e {
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			case 'r':
				b.WriteByte('\r')
			case '\\':
				b.WriteByte('\\')
			case '\'':
				b.WriteByte('\'')
			case '"':
				b.WriteByte('"')
			case 'x':
				if p.i+2 >= len(p.s) {
					return "", false
				}
				hv, err := strconv.ParseUint(p.s[p.i+1:p.i+3], 16, 8)
				if err != nil {
					return "", false
				}
				b.WriteByte(byte(hv))
				p.i += 2
			default:
				return "", false
			}
			p.i++
			continue
		}
		b.WriteByte(c)
		p.i++
	}
	return "", false
}

// escapePyString re-escapes decoded string contents for the canonical
// double-quoted form.
func escapePyString(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		case '\n':
			b.WriteString(`\n`)
		case '\t':
			b.WriteString(`\t`)
		case '\r':
			b.WriteString(`\r`)
		default:
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

var pyNumberRe = regexp.MustCompile(`^[+-]?(\d+(\.\d*)?|\.\d+)([eE][+-]?\d+)?`)

// parseNumber reads an int or float literal. The canonical form keeps the
// token verbatim (1.0 is not 1 as a literal) except for a leading plus.
func (p *pyParser) parseNumber() (string, bool) {
	m := pyNumberRe.FindString(p.s[p.i:])
	if m == "" {
		return "", false
	}
	p.i += len(m)
	return strings.TrimPrefix(m, "+"), true
}

// parseKeyword reads True/False/None case-insensitively.
func (p *pyParser) parseKeyword() (string, bool) {
	start := p.i
	for p.i < len(p.s) && (p.s[p.i] >= 'a' && p.s[p.i] <= 'z' || p.s[p.i] >= 'A' && p.s[p.i] <= 'Z') {
		p.i++
	}
	switch strings.ToLower(p.s[start:p.i]) {
	case "true":
		return "True", true
	case "false":
		return "False", true
	case "none":
		return "None", true
	}
	return "", false
}

func (p *pyParser) parseList() (string, bool) {
	p.i++ // [
	elems, ok := p.parseElems(']')
	if !ok {
		return "", false
	}
	return "[" + strings.Join(elems, ",") + "]", true
}

// parseTuple reads a parenthesized form following Python semantics
// (ast.literal_eval): "(v)" is bare grouping and canonicalizes to the value
// itself, "(v,)" is a single-element tuple, "(a, b)" a tuple. Confusing
// grouping for a 1-tuple would let a wrong answer score a hit, so the
// distinction is exact.
func (p *pyParser) parseTuple() (string, bool) {
	p.i++ // (
	p.skipWS()
	if p.i < len(p.s) && p.s[p.i] == ')' {
		p.i++
		return "()", true
	}
	first, ok := p.parseValue()
	if !ok {
		return "", false
	}
	p.skipWS()
	if p.i < len(p.s) && p.s[p.i] == ')' {
		p.i++
		return first, true
	}
	if p.i >= len(p.s) || p.s[p.i] != ',' {
		return "", false
	}
	p.i++
	rest, ok := p.parseElems(')')
	if !ok {
		return "", false
	}
	elems := append([]string{first}, rest...)
	if len(elems) == 1 {
		return "(" + elems[0] + ",)", true
	}
	return "(" + strings.Join(elems, ",") + ")", true
}

// parseElems reads comma-separated values up to close, allowing one trailing
// comma.
func (p *pyParser) parseElems(close byte) ([]string, bool) {
	var elems []string
	p.skipWS()
	if p.i < len(p.s) && p.s[p.i] == close {
		p.i++
		return elems, true
	}
	for {
		v, ok := p.parseValue()
		if !ok {
			return nil, false
		}
		elems = append(elems, v)
		p.skipWS()
		if p.i >= len(p.s) {
			return nil, false
		}
		if p.s[p.i] == ',' {
			p.i++
			p.skipWS()
			if p.i < len(p.s) && p.s[p.i] == close {
				p.i++
				return elems, true
			}
			continue
		}
		if p.s[p.i] == close {
			p.i++
			return elems, true
		}
		return nil, false
	}
}

// parseDictOrSet reads a dict or set literal. Empty {} is a dict. Entries and
// elements are sorted by canonical form so order-insensitive Python ==
// semantics hold for both.
func (p *pyParser) parseDictOrSet() (string, bool) {
	p.i++ // {
	p.skipWS()
	if p.i < len(p.s) && p.s[p.i] == '}' {
		p.i++
		return "{}", true
	}
	type pair struct{ k, v string }
	var pairs []pair
	var elems []string
	isDict := false
	first := true
	closed := false
	for !closed {
		k, ok := p.parseValue()
		if !ok {
			return "", false
		}
		p.skipWS()
		if first {
			first = false
			isDict = p.i < len(p.s) && p.s[p.i] == ':'
		}
		if isDict {
			if p.i >= len(p.s) || p.s[p.i] != ':' {
				return "", false
			}
			p.i++
			v, ok := p.parseValue()
			if !ok {
				return "", false
			}
			pairs = append(pairs, pair{k, v})
		} else {
			elems = append(elems, k)
		}
		p.skipWS()
		if p.i >= len(p.s) {
			return "", false
		}
		switch p.s[p.i] {
		case ',':
			p.i++
			p.skipWS()
			if p.i < len(p.s) && p.s[p.i] == '}' {
				p.i++
				closed = true
			}
		case '}':
			p.i++
			closed = true
		default:
			return "", false
		}
	}
	if isDict {
		sort.Slice(pairs, func(a, b int) bool {
			if pairs[a].k != pairs[b].k {
				return pairs[a].k < pairs[b].k
			}
			return pairs[a].v < pairs[b].v
		})
		parts := make([]string, len(pairs))
		for i, pr := range pairs {
			parts[i] = pr.k + ":" + pr.v
		}
		return "{" + strings.Join(parts, ",") + "}", true
	}
	sort.Strings(elems)
	return "{" + strings.Join(elems, ",") + "}", true
}
