// Package imageparams holds the matching logic for image-probe cost-saving
// parameter rules (spec 0014 US 16-18 / GH #33). Probe request bodies default
// to the minimal shape {model, prompt, n:1} — the most portable form across
// upstreams — and rules keyed on the model ID append extra parameters (the
// seeded rule sends quality:"low" to gpt-image models, cutting the per-image
// probe cost to roughly $0.011). Rules are stored in the database and managed
// through the admin UI; this package holds the rule type, the merge logic,
// and the default rule set seeded on first run.
package imageparams

import "strings"

// Rule matches a model ID containing Keyword (case-insensitive substring) and
// contributes Params to the image probe request body. Param values are
// strings only (v1); both images_generation (JSON body) and images_edit
// (multipart fields) consume the same merged map.
type Rule struct {
	ID       int64
	Keyword  string
	Params   map[string]string
	Priority int
}

// reservedKeys are the request-body keys owned by the probe contract itself.
// Rules may never set them: the API rejects them at write time and Merge
// skips them defensively, so even a hand-edited database cannot override the
// model being probed or the fixed prompt.
var reservedKeys = map[string]bool{"model": true, "prompt": true, "n": true}

// IsReservedKey reports whether key is owned by the probe contract
// (case-insensitive).
func IsReservedKey(key string) bool {
	return reservedKeys[strings.ToLower(key)]
}

// Matches reports whether the rule's keyword appears in the model ID
// (case-insensitive substring).
func (r Rule) Matches(modelID string) bool {
	return strings.Contains(strings.ToLower(modelID), strings.ToLower(r.Keyword))
}

// Merge resolves the extra probe parameters for a model against the rule set
// (ordered by ascending priority then id — the store's list order). Every
// matching rule contributes its params; on a key collision the rule with the
// smaller priority number wins. Reserved keys are skipped defensively (the
// API already rejects them at write time — double insurance). A nil or empty
// rule set, or no match, yields nil: the minimal request body.
func Merge(modelID string, rules []Rule) map[string]string {
	var out map[string]string
	for _, r := range rules {
		if !r.Matches(modelID) {
			continue
		}
		for k, v := range r.Params {
			if IsReservedKey(k) {
				continue
			}
			if _, exists := out[k]; exists {
				continue // an earlier (lower-priority-number) rule set this key
			}
			if out == nil {
				out = map[string]string{}
			}
			out[k] = v
		}
	}
	return out
}

// DefaultRules is the built-in rule set seeded into the database exactly once
// on first run. Afterwards the database is the source of truth; this list
// only covers first boot. The gpt-image family rule is the cost-saving core
// of spec 0014: low quality is roughly $0.011/image versus several cents at
// higher tiers.
func DefaultRules() []Rule {
	return []Rule{
		{Keyword: "gpt-image", Params: map[string]string{"quality": "low"}, Priority: 100},
	}
}
