// Package status implements the Endpoint health state machine. Given an
// endpoint's probe history it derives one of four states (healthy, degraded,
// down, failing) plus a human-readable Chinese reason, following the rules in
// api-contract.md:
//
//	down     — the last 3 consecutive probes failed
//	failing  — the latest probe failed but fewer than 3 in a row
//	degraded — 24h success rate < 0.95, or 24h P95 latency exceeds 2x the
//	           7-day P50 baseline (skipped when the baseline is too small)
//	healthy  — everything else
package status

import (
	"fmt"
	"math"
	"sort"
)

// Kind is one of the four endpoint health states.
type Kind string

const (
	KindHealthy  Kind = "healthy"
	KindDegraded Kind = "degraded"
	KindDown     Kind = "down"
	KindFailing  Kind = "failing"
)

// downThreshold is the number of consecutive failures that marks an
// endpoint as down.
const downThreshold = 3

// minSuccessRate is the minimum acceptable 24h success rate.
const minSuccessRate = 0.95

// latencyDegradeFactor is how many times the 24h P95 may exceed the 7-day
// P50 baseline before the endpoint is considered degraded.
const latencyDegradeFactor = 2.0

// MinBaselineSamples is the minimum number of probes inside the 7-day
// window required for the latency baseline to be meaningful. Below this the
// latency degradation check is skipped.
const MinBaselineSamples = 5

// Sample is the minimal probe data needed for window statistics.
type Sample struct {
	OK        bool
	LatencyMs int
}

// Input carries everything Evaluate needs about one endpoint.
type Input struct {
	// TotalProbes is the endpoint's overall probe count (0 = never probed).
	TotalProbes int
	// ConsecutiveFailures counts failures since the most recent success.
	ConsecutiveFailures int
	// LastError is the error summary of the most recent failed probe.
	LastError string
	// Samples24h holds all probes inside the last 24 hours.
	Samples24h []Sample
	// BaselineP50Ms is the 7-day P50 latency baseline in milliseconds.
	BaselineP50Ms float64
	// HasBaseline reports whether BaselineP50Ms is trustworthy.
	HasBaseline bool
}

// Result is the evaluated state plus its human-readable justification.
type Result struct {
	Kind   Kind
	Reason string
}

// Percentile returns the nearest-rank percentile of the given latencies.
// It returns 0 for an empty input and never mutates the caller's slice.
func Percentile(latencies []int, p float64) float64 {
	if len(latencies) == 0 {
		return 0
	}
	sorted := make([]int, len(latencies))
	copy(sorted, latencies)
	sort.Ints(sorted)
	rank := int(math.Ceil(p / 100 * float64(len(sorted))))
	if rank < 1 {
		rank = 1
	}
	return float64(sorted[rank-1])
}

// SuccessRate computes the ok ratio of the given samples. The second return
// value reports whether any samples existed at all.
func SuccessRate(samples []Sample) (float64, bool) {
	if len(samples) == 0 {
		return 0, false
	}
	ok := 0
	for _, s := range samples {
		if s.OK {
			ok++
		}
	}
	return float64(ok) / float64(len(samples)), true
}

// Latencies extracts the latency values from samples for percentile math.
func Latencies(samples []Sample) []int {
	out := make([]int, 0, len(samples))
	for _, s := range samples {
		out = append(out, s.LatencyMs)
	}
	return out
}

// Evaluate applies the four status rules in priority order.
func Evaluate(in Input) Result {
	lastError := in.LastError
	if lastError == "" {
		lastError = "未知错误"
	}

	// Rule 1: three or more consecutive failures means the endpoint is down.
	if in.ConsecutiveFailures >= downThreshold {
		return Result{
			Kind:   KindDown,
			Reason: fmt.Sprintf("连续 %d 次失败,最近错误: %s", in.ConsecutiveFailures, lastError),
		}
	}

	// Rule 2: the latest probe failed but the down threshold is not reached.
	if in.ConsecutiveFailures > 0 {
		return Result{
			Kind:   KindFailing,
			Reason: fmt.Sprintf("连续 %d 次失败,最近错误: %s", in.ConsecutiveFailures, lastError),
		}
	}

	// Rule 3a: 24h success rate below the threshold.
	if rate, ok := SuccessRate(in.Samples24h); ok && rate < minSuccessRate {
		return Result{
			Kind:   KindDegraded,
			Reason: fmt.Sprintf("24h 成功率 %.1f%% 低于 95%%", rate*100),
		}
	}

	// Rule 3b: 24h P95 latency above twice the 7-day P50 baseline.
	if in.HasBaseline && len(in.Samples24h) > 0 {
		p95 := Percentile(Latencies(in.Samples24h), 95)
		if p95 > latencyDegradeFactor*in.BaselineP50Ms {
			return Result{
				Kind: KindDegraded,
				Reason: fmt.Sprintf("P95 延迟 %.1fs 超过基线 2 倍(基线 %.1fs)",
					p95/1000, in.BaselineP50Ms/1000),
			}
		}
	}

	// Rule 4: everything else is healthy.
	if in.TotalProbes == 0 {
		return Result{Kind: KindHealthy, Reason: "暂无探测数据"}
	}
	return Result{Kind: KindHealthy, Reason: "运行正常"}
}
