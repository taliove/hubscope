package evaluator

import (
	"strings"

	"golang.org/x/text/unicode/norm"

	"github.com/taliove/hubscope/internal/store"
)

// VerdictProfileCurrent is the scoring caliber every new rule verdict uses
// (ADR 0008). Bump it — never edit an old profile — when the pipeline
// changes, so historical runs stay reproducible and cross-time comparability
// breaks are explicit.
const VerdictProfileCurrent = store.VerdictProfileV2

// quotePairClose returns the closing counterpart of an opening quote we
// strip: straight and curly English quotes plus the Chinese corner brackets.
// ok is false for any other rune.
func quotePairClose(open rune) (close rune, ok bool) {
	switch open {
	case '"':
		return '"', true
	case '\'':
		return '\'', true
	case '“':
		return '”', true
	case '‘':
		return '’', true
	case '「':
		return '」', true
	case '『':
		return '』', true
	}
	return 0, false
}

// normalizeForVerdict dispatches the answer/expected normalization on the
// verdict profile. Unknown profiles fall back to v1, the narrowest caliber.
func normalizeForVerdict(profile, s string) string {
	if profile == store.VerdictProfileV2 {
		return normalizeV2(s)
	}
	return normalizeV1(s)
}

// normalizeV1 is the legacy caliber: TrimSpace only.
func normalizeV1(s string) string {
	return strings.TrimSpace(s)
}

// normalizeV2 is the full pipeline (ADR 0008): trim, strip paired quotes,
// Unicode NFKC (full/half-width folding), then collapse inner whitespace
// runs to a single space. Case is preserved on purpose — strictness is part
// of instruction-following scores. Quotes are stripped again after NFKC
// because compatibility folding can map full-width quotes onto straight ones.
func normalizeV2(s string) string {
	s = stripPairedQuotes(strings.TrimSpace(s))
	s = norm.NFKC.String(s)
	s = stripPairedQuotes(s)
	return strings.Join(strings.Fields(s), " ")
}

// stripPairedQuotes removes outer paired quote layers (repeatedly, so
// «"e"»-style double wrapping unfolds), trimming between layers.
// Unbalanced or single quotes are left untouched.
func stripPairedQuotes(s string) string {
	for {
		s = strings.TrimSpace(s)
		runes := []rune(s)
		if len(runes) < 2 {
			return s
		}
		if closer, ok := quotePairClose(runes[0]); !ok || closer != runes[len(runes)-1] {
			return s
		}
		s = string(runes[1 : len(runes)-1])
	}
}
