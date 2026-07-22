package status

import (
	"fmt"
	"math"
)

// Score caps mirroring the status machine severity ladder.
const (
	scoreCapDown     = 20
	scoreCapFailing  = 50
	scoreCapDegraded = 80
)

// latencyDegradeDeduction is the fixed deduction when the 24h P95 latency
// exceeds twice the baseline.
const latencyDegradeDeduction = 15

// ScoreResult is the deterministic 0-100 health score of an endpoint plus
// the human-readable Chinese justifications for every deduction and cap.
type ScoreResult struct {
	// Value is the final score, meaningful only when HasScore is true.
	Value int
	// HasScore is false when the endpoint was never probed.
	HasScore bool
	// Reasons lists each deduction and cap in application order.
	Reasons []string
}

// Score derives a deterministic 0-100 score from the same inputs as the
// status machine: start at 100, deduct for a low 24h success rate and for
// latency degradation, then cap by failure streak and degradation, finally
// clamp into [0,100].
func Score(in Input) ScoreResult {
	if in.TotalProbes == 0 {
		return ScoreResult{HasScore: false, Reasons: []string{}}
	}

	score := 100
	reasons := []string{}
	degraded := false

	// Deduction 1: 24h success rate below the threshold, proportional to
	// the gap (one point per percentage point below 95%). A gap that rounds
	// to zero points is recorded only through the degraded cap — a "扣 0 分"
	// reason would read as noise.
	if rate, ok := SuccessRate(in.Samples24h); ok && rate < minSuccessRate {
		degraded = true
		deduction := int(math.Round((minSuccessRate - rate) * 100))
		score -= deduction
		if deduction > 0 {
			reasons = append(reasons, fmt.Sprintf("24h 成功率 %.1f%%,扣 %d 分", rate*100, deduction))
		}
	}

	// Deduction 2: 24h P95 latency above twice the baseline, fixed -15.
	if in.HasBaseline && len(in.Samples24h) > 0 {
		p95 := Percentile(Latencies(in.Samples24h), 95)
		if p95 > latencyDegradeFactor*in.BaselineP50Ms {
			degraded = true
			score -= latencyDegradeDeduction
			reasons = append(reasons, fmt.Sprintf("P95 延迟 %.0f ms 超基线 2 倍,扣 %d 分", p95, latencyDegradeDeduction))
		}
	}

	// Caps follow the status machine priority: down, failing, degraded.
	switch {
	case in.ConsecutiveFailures >= DownThreshold:
		score = min(score, scoreCapDown)
		reasons = append(reasons, fmt.Sprintf("连续 %d 次失败,封顶 %d 分", in.ConsecutiveFailures, scoreCapDown))
	case in.ConsecutiveFailures > 0:
		score = min(score, scoreCapFailing)
		reasons = append(reasons, fmt.Sprintf("连续 %d 次失败,封顶 %d 分", in.ConsecutiveFailures, scoreCapFailing))
	case degraded:
		score = min(score, scoreCapDegraded)
		reasons = append(reasons, fmt.Sprintf("性能降级,封顶 %d 分", scoreCapDegraded))
	}

	score = max(score, 0)
	score = min(score, 100)
	return ScoreResult{Value: score, HasScore: true, Reasons: reasons}
}
