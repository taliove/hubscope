// Package classifier maps a model ID to its classification along two
// dimensions: capability (chat/embedding/image/audio/rerank/moderation/...)
// and family (gpt/claude/qwen/deepseek/... vendor series). Rules are stored
// in the database and managed through the admin UI; this package holds the
// matching logic and the default rule set seeded on first run.
package classifier

import "strings"

// Dimensions supported by classification rules.
const (
	DimensionCapability = "capability"
	DimensionFamily     = "family"
)

// Fallback categories when no rule matches.
const (
	DefaultCapability = "chat"
	DefaultFamily     = "other"
)

// Rule matches a model ID containing Keyword (case-insensitive) and assigns
// Category on the rule's Dimension. Rules are evaluated in ascending
// (Priority, ID) order per dimension; the first match wins.
type Rule struct {
	ID        int64
	Dimension string
	Keyword   string
	Category  string
	Priority  int
}

// Matches reports whether the rule's keyword appears in the model ID.
func (r Rule) Matches(modelID string) bool {
	return strings.Contains(strings.ToLower(modelID), strings.ToLower(r.Keyword))
}

// Classify resolves both dimensions of a model ID against the rule set,
// falling back to the defaults per dimension.
func Classify(modelID string, rules []Rule) (capability, family string) {
	return classifyOn(DimensionCapability, modelID, rules, DefaultCapability),
		classifyOn(DimensionFamily, modelID, rules, DefaultFamily)
}

// classifyOn resolves one dimension: the category of the first matching rule
// in the given order, or the fallback.
func classifyOn(dimension, modelID string, rules []Rule, fallback string) string {
	for _, r := range rules {
		if r.Dimension == dimension && r.Matches(modelID) {
			return r.Category
		}
	}
	return fallback
}

// DefaultRules is the built-in rule set seeded into the database on first
// run. Afterwards the database is the source of truth; this list only covers
// first boot. Keywords are matched case-insensitively as substrings.
func DefaultRules() []Rule {
	type entry struct{ dimension, keyword, category string }
	entries := []entry{
		// Capability: non-conversational model kinds first.
		{DimensionCapability, "embedding", "embedding"},
		{DimensionCapability, "bge", "embedding"},
		{DimensionCapability, "gte-", "embedding"},
		{DimensionCapability, "-e5", "embedding"},
		{DimensionCapability, "m3e", "embedding"},
		{DimensionCapability, "text2vec", "embedding"},
		{DimensionCapability, "image", "image"},
		{DimensionCapability, "dall", "image"},
		{DimensionCapability, "flux", "image"},
		{DimensionCapability, "stable-diffusion", "image"},
		{DimensionCapability, "video", "video"},
		{DimensionCapability, "seedance", "video"},
		{DimensionCapability, "tts", "audio"},
		{DimensionCapability, "whisper", "audio"},
		{DimensionCapability, "audio", "audio"},
		{DimensionCapability, "speech", "audio"},
		{DimensionCapability, "rerank", "rerank"},
		{DimensionCapability, "moderation", "moderation"},

		// Family: vendor / model series. "gpt-" carries the dash so GPTQ
		// quantization suffixes (llama-3-8b-gptq) do not misfire.
		{DimensionFamily, "gpt-", "gpt"},
		{DimensionFamily, "openai", "gpt"},
		{DimensionFamily, "o1-", "gpt"},
		{DimensionFamily, "o3-", "gpt"},
		{DimensionFamily, "o4-", "gpt"},
		{DimensionFamily, "claude", "claude"},
		{DimensionFamily, "gemini", "gemini"},
		{DimensionFamily, "qwen", "qwen"},
		{DimensionFamily, "qwq", "qwen"},
		{DimensionFamily, "deepseek", "deepseek"},
		{DimensionFamily, "glm", "glm"},
		{DimensionFamily, "llama", "llama"},
		{DimensionFamily, "mistral", "mistral"},
		{DimensionFamily, "mixtral", "mistral"},
		{DimensionFamily, "doubao", "doubao"},
		{DimensionFamily, "kimi", "kimi"},
		{DimensionFamily, "moonshot", "kimi"},
		{DimensionFamily, "grok", "grok"},
		{DimensionFamily, "hunyuan", "hunyuan"},
		{DimensionFamily, "minimax", "minimax"},
		{DimensionFamily, "baichuan", "baichuan"},
		{DimensionFamily, "cohere", "cohere"},
		{DimensionFamily, "command-", "cohere"},
		{DimensionFamily, "ernie", "ernie"},
		{DimensionFamily, "spark", "spark"},
		{DimensionFamily, "yi-", "yi"},
		{DimensionFamily, "phi-", "phi"},
	}

	rules := make([]Rule, 0, len(entries))
	for _, e := range entries {
		rules = append(rules, Rule{
			Dimension: e.dimension,
			Keyword:   e.keyword,
			Category:  e.category,
			Priority:  100,
		})
	}
	return rules
}
