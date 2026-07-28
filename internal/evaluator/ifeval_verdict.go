package evaluator

import (
	"fmt"
	"strings"

	"github.com/taliove/hubscope/internal/evaluator/ifeval"
)

// ifevalVerdict scores an answer against the case's IFEval check params
// (ticket 97, spec 0014 decision C): every instruction in the params runs
// its ported official checker, and the score is all-or-nothing — 1 when all
// are followed, 0 when any fails. A miss is still a scored verdict (the
// model did answer), so the score is never nil for a well-formed case; a
// malformed check_params is a caliber bug and yields a nil score (the
// invalid-regex precedent: broken configuration is not a model failure).
func ifevalVerdict(checkParams *string, answer string) (*float64, string) {
	if checkParams == nil {
		return nil, "rule error: ifeval case missing check_params"
	}
	total, failed, err := ifeval.Check(*checkParams, answer)
	if err != nil {
		return nil, "rule error: " + err.Error()
	}
	if len(failed) == 0 {
		return scorePtr(1), fmt.Sprintf("rule ifeval: all %d instructions followed", total)
	}
	return scorePtr(0), fmt.Sprintf("rule ifeval: %d/%d instructions followed, failed: %s",
		total-len(failed), total, strings.Join(failed, ", "))
}
