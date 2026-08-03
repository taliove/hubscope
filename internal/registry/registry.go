// Package registry holds the built-in table of model characteristics and
// pricing (spec 0020 ticket 1): an IQ tier (1–10) and per-token list
// prices keyed by model ID pattern. It serves two consumers — jury
// selection (evaluator) and eval cost estimation — and is a pure query
// module: no I/O, no state, no clock.
//
// Prices are public list-price snapshots in USD per 1M tokens; they go
// stale between releases, so administrators can correct them or register
// unknown models through the model_registry_overrides setting. Override
// entries merge field-by-field over the built-in table. An unregistered
// model reports nil fields: callers must render that honestly ("price not
// registered") and never treat nil as zero.
package registry

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Info is the effective characteristics of one model.
type Info struct {
	IQ       *float64 // 1–10 quality tier; nil = unregistered
	PriceIn  *float64 // USD per 1M input tokens; nil = price not registered
	PriceOut *float64 // USD per 1M output tokens; nil = price not registered
}

// Override is one administrator-registered entry, field-merged over the
// built-in table for the models its pattern matches.
type Override struct {
	Match    string   `json:"match"`
	IQ       *float64 `json:"iq_tier,omitempty"`
	PriceIn  *float64 `json:"price_in,omitempty"`
	PriceOut *float64 `json:"price_out,omitempty"`
}

type entry struct {
	pattern string
	info    Info
}

// builtin is the shipped snapshot. Patterns are exact IDs or prefixes with
// a trailing '*'; exact matches beat prefixes, longer prefixes beat
// shorter ones.
var builtin = []entry{
	{"claude-opus-4-*", Info{new(9.5), new(15.0), new(75.0)}},
	{"claude-sonnet-4-*", Info{new(8.5), new(3.0), new(15.0)}},
	{"claude-haiku-4-*", Info{new(7.0), new(1.0), new(5.0)}},
	{"gpt-4o-mini*", Info{new(7.0), new(0.15), new(0.6)}},
	{"gpt-4o*", Info{new(8.0), new(2.5), new(10.0)}},
	{"o3*", Info{new(9.5), new(10.0), new(40.0)}},
	{"deepseek-r1*", Info{new(9.0), new(0.55), new(2.19)}},
	{"deepseek-v3*", Info{new(8.5), new(0.27), new(1.1)}},
	{"qwen3-235b*", Info{new(9.0), new(2.0), new(8.0)}},
	{"qwen3-32b*", Info{new(7.5), new(0.3), new(1.0)}},
	{"qwen3-30b-a3b*", Info{new(7.5), new(0.3), new(1.0)}},
	{"qwen2.5-72b*", Info{new(7.0), new(0.35), new(1.4)}},
	{"glm-4-plus*", Info{new(8.0), new(0.5), new(0.5)}},
	{"glm-4-flash*", Info{new(6.0), new(0.1), new(0.1)}},
	{"llama-3.1-405b*", Info{new(8.0), new(3.0), new(3.0)}},
	{"llama-3.1-70b*", Info{new(7.0), new(0.35), new(0.4)}},
	{"llama-3.1-8b*", Info{new(5.5), new(0.05), new(0.08)}},
	{"mistral-large*", Info{new(8.5), new(2.0), new(6.0)}},
	{"gemini-2.5-pro*", Info{new(9.0), new(1.25), new(10.0)}},
	{"gemini-2.5-flash*", Info{new(7.5), new(0.3), new(2.5)}},
	{"yi-lightning*", Info{new(6.5), new(0.1), new(0.1)}},
	{"kimi-k2*", Info{new(8.5), new(0.6), new(2.5)}},
}

// Lookup returns the effective info for a model ID: the best-matching
// override merged field-by-field over the best-matching built-in entry.
// A field the override leaves nil keeps the built-in value; a model no
// entry matches reports an all-nil Info.
func Lookup(modelID string, overrides []Override) Info {
	var merged Info
	if bi := bestMatch(modelID, builtin); bi != nil {
		merged = bi.info
	}
	if ov := bestOverride(modelID, overrides); ov != nil {
		if ov.IQ != nil {
			merged.IQ = ov.IQ
		}
		if ov.PriceIn != nil {
			merged.PriceIn = ov.PriceIn
		}
		if ov.PriceOut != nil {
			merged.PriceOut = ov.PriceOut
		}
	}
	return merged
}

// bestMatch finds the winning entry for id: an exact pattern beats any
// prefix, a longer prefix beats a shorter one.
func bestMatch(id string, entries []entry) *entry {
	var best *entry
	bestLen := -1
	for i := range entries {
		p := entries[i].pattern
		if pre, ok := strings.CutSuffix(p, "*"); ok {
			if strings.HasPrefix(id, pre) && len(pre) > bestLen {
				best, bestLen = &entries[i], len(pre)
			}
			continue
		}
		if p == id {
			return &entries[i] // exact wins outright
		}
	}
	return best
}

// bestOverride applies the same matching discipline to administrator
// overrides, returning the winning entry's info.
func bestOverride(id string, overrides []Override) *Info {
	entries := make([]entry, len(overrides))
	for i, ov := range overrides {
		entries[i] = entry{pattern: ov.Match, info: Info{IQ: ov.IQ, PriceIn: ov.PriceIn, PriceOut: ov.PriceOut}}
	}
	best := bestMatch(id, entries)
	if best == nil {
		return nil
	}
	return &best.info
}

// ParseOverrides decodes the stored overrides JSON. An empty string yields
// no overrides; malformed JSON is an error so callers can fall back
// explicitly (reads fail open to the built-in table).
func ParseOverrides(raw string) ([]Override, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var ovs []Override
	if err := json.Unmarshal([]byte(raw), &ovs); err != nil {
		return nil, fmt.Errorf("parse model registry overrides: %w", err)
	}
	return ovs, nil
}

// MaxOverrides bounds the override list so a pasted blob cannot grow the
// settings row without limit.
const MaxOverrides = 200

// maxPatternLen bounds one match pattern.
const maxPatternLen = 128

// maxPrice sanity-caps a per-1M-token price in USD.
const maxPrice = 10000

// Validate checks overrides at the write boundary: non-empty bounded
// patterns, IQ inside [1,10], prices inside [0, maxPrice], and at least
// one valued field per entry.
func Validate(overrides []Override) error {
	if len(overrides) > MaxOverrides {
		return fmt.Errorf("at most %d override entries", MaxOverrides)
	}
	for i, ov := range overrides {
		if strings.TrimSpace(ov.Match) == "" {
			return fmt.Errorf("entry %d: match must not be empty", i)
		}
		if len(ov.Match) > maxPatternLen {
			return fmt.Errorf("entry %d: match longer than %d characters", i, maxPatternLen)
		}
		if ov.IQ != nil && (*ov.IQ < 1 || *ov.IQ > 10) {
			return fmt.Errorf("entry %d (%s): iq_tier must be between 1 and 10", i, ov.Match)
		}
		for name, p := range map[string]*float64{"price_in": ov.PriceIn, "price_out": ov.PriceOut} {
			if p != nil && (*p < 0 || *p > maxPrice) {
				return fmt.Errorf("entry %d (%s): %s must be between 0 and %d", i, ov.Match, name, maxPrice)
			}
		}
		if ov.IQ == nil && ov.PriceIn == nil && ov.PriceOut == nil {
			return errors.New("entry " + ov.Match + ": at least one field must be set")
		}
	}
	return nil
}
