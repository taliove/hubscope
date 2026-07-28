package evaluator

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/taliove/hubscope/internal/store"
)

// ruleVerdict scores an answer by the case's rule mode: hit is 1, miss is 0.
// exact/contains normalize both sides through the verdict profile pipeline
// (ADR 0008) so the comparison stays symmetric; regex is never normalized —
// the pattern itself is the caliber.
func ruleVerdict(c store.Case, answer, profile string) (*float64, string) {
	mode := ""
	expected := ""
	if c.RuleMode != nil {
		mode = *c.RuleMode
	}
	if c.RuleExpected != nil {
		expected = *c.RuleExpected
	}

	// mcq has its own extraction-and-match caliber (ADR 0013) and reports
	// extraction failures distinctly, so it bypasses the generic switch.
	if mode == "mcq" {
		return mcqVerdict(expected, answer)
	}
	// numeric likewise (ticket 95): ####-marker/last-line extraction with
	// separator/sign/decimal normalization, conservative on failure.
	if mode == "numeric" {
		return numericVerdict(expected, answer)
	}

	hit := false
	switch mode {
	case "exact":
		hit = normalizeForVerdict(profile, answer) == normalizeForVerdict(profile, expected)
	case "contains":
		hit = strings.Contains(normalizeForVerdict(profile, answer), normalizeForVerdict(profile, expected))
	case "regex":
		re, err := regexp.Compile(expected)
		if err != nil {
			return nil, fmt.Sprintf("rule error: invalid regex %q: %v", expected, err)
		}
		hit = re.MatchString(answer)
	default:
		return nil, fmt.Sprintf("rule error: unknown mode %q", mode)
	}

	if hit {
		return scorePtr(1), fmt.Sprintf("rule %s matched (expected %q)", mode, expected)
	}
	return scorePtr(0), fmt.Sprintf("rule %s not matched (expected %q)", mode, expected)
}

// judgeVerdict calls the judge model through the same hub/protocol as the
// evaluated model and parses its JSON verdict. A judge call failure or an
// unparseable judge response yields a nil score (unjudged), never a 0.
func (e *Evaluator) judgeVerdict(ctx context.Context, hub *store.Hub, protocol, judgeModel string, c store.Case, answer string) (*float64, string) {
	res := e.client.Complete(ctx, hub.BaseURL, hub.Token, protocol, judgeModel, buildJudgePrompt(c, answer), evalMaxTokens)
	if !res.OK {
		detail := "judge call failed"
		if res.ErrorSummary != nil {
			detail = "judge call failed: " + *res.ErrorSummary
		}
		return nil, detail
	}

	score, reason, err := parseJudgeResponse(res.Text)
	if err != nil {
		return nil, fmt.Sprintf("judge parse failed: %v (raw: %s)", err, truncateRunes(res.Text, 200))
	}
	return scorePtr(score), "judge: " + reason
}

// buildJudgePrompt assembles rubric + question + answer for the judge.
func buildJudgePrompt(c store.Case, answer string) string {
	rubric := ""
	if c.Rubric != nil {
		rubric = *c.Rubric
	}
	return fmt.Sprintf(
		"你是评估裁判。请按以下评分标准给「作答」打分。\n\n评分标准:\n%s\n\n题目:\n%s\n\n作答:\n%s\n\n请只输出 JSON:{\"score\": 0到1之间的数字, \"reason\": \"简短中文理由\"},不要输出任何其他内容。",
		rubric, c.Prompt, answer,
	)
}

// judgeResponse is the JSON shape the judge is asked to produce.
type judgeResponse struct {
	Score  float64 `json:"score"`
	Reason string  `json:"reason"`
}

// parseJudgeResponse extracts {"score","reason"} from the judge's reply,
// tolerating surrounding prose by slicing out the outermost JSON object.
func parseJudgeResponse(text string) (float64, string, error) {
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start < 0 || end <= start {
		return 0, "", fmt.Errorf("no JSON object found")
	}

	var parsed judgeResponse
	if err := json.Unmarshal([]byte(text[start:end+1]), &parsed); err != nil {
		return 0, "", fmt.Errorf("invalid judge JSON: %w", err)
	}

	// Clamp to the documented 0~1 range; judges sometimes overshoot.
	if parsed.Score < 0 {
		parsed.Score = 0
	}
	if parsed.Score > 1 {
		parsed.Score = 1
	}
	return parsed.Score, parsed.Reason, nil
}

// scorePtr returns a pointer to s.
func scorePtr(s float64) *float64 { return &s }

// truncateRunes limits s to n runes for inclusion in verdict details.
func truncateRunes(s string, n int) string {
	runes := []rune(s)
	if len(runes) > n {
		return string(runes[:n]) + "…"
	}
	return s
}
