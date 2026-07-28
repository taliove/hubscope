package server

import (
	"net/http"
	"sort"
	"time"

	"github.com/taliove/hubscope/internal/status"
	"github.com/taliove/hubscope/internal/store"
)

// overviewWindows defines the lookback windows used by the status machine.
const (
	overviewWindow24h    = 24 * time.Hour
	overviewBaselineSpan = 7 * 24 * time.Hour
)

// overviewDTO is the response body of GET /api/overview.
type overviewDTO struct {
	GeneratedAt  string             `json:"generated_at"`
	Endpoints    []overviewEntryDTO `json:"endpoints"`
	ByFamily     []overviewGroupDTO `json:"by_family"`
	ByCapability []overviewGroupDTO `json:"by_capability"`
	ByProtocol   []overviewGroupDTO `json:"by_protocol"`
	// Global aggregates (ticket 36): EnabledEndpoints counts only enabled
	// endpoints; Availability24h is the probe-weighted 24h availability
	// across all enabled endpoints (total successful probes over total
	// probes, the same weighting as the per-group availability), null when
	// no enabled endpoint has probes in the window.
	EnabledEndpoints int      `json:"enabled_endpoints"`
	Availability24h  *float64 `json:"availability_24h"`
}

// overviewEntryDTO is the per-endpoint status summary. Field names follow
// the api-contract.md Overview section exactly.
type overviewEntryDTO struct {
	EndpointID   int64  `json:"endpoint_id"`
	ModelID      string `json:"model_id"`
	Protocol     string `json:"protocol"`
	Enabled      bool   `json:"enabled"`
	Status       string `json:"status"`
	StatusReason string `json:"status_reason"`
	// DegradeCauses lists the structured degrade causes ("availability",
	// "latency"); always serialized as an array, empty unless degraded.
	DegradeCauses  []string         `json:"degrade_causes"`
	SuccessRate24h *float64         `json:"success_rate_24h"`
	P50Ms          *float64         `json:"p50_ms"`
	P95Ms          *float64         `json:"p95_ms"`
	LastProbeAt    *string          `json:"last_probe_at"`
	Family         string           `json:"family"`
	Capability     string           `json:"capability"`
	Score          *int             `json:"score"`
	ScoreReasons   []string         `json:"score_reasons"`
	Dots24h        []overviewDotDTO `json:"dots_24h"`
	EvalScore      *float64         `json:"eval_score"`
	// BaselineP50Ms passes through the status machine's own 7-day P50
	// baseline (the value the latency degradation rule compares against);
	// null when the baseline has too few samples. Never recomputed here.
	BaselineP50Ms *float64 `json:"baseline_p50_ms"`
}

// overviewDotDTO is one hourly bucket of the 24h stability dots: how many
// probes ran in that hour and how many of them failed, plus the P50 latency
// of the bucket's SUCCESSFUL probes only — a failed probe's latency is
// time-to-failure, not service latency, so it must never pollute the
// sparkline (null when the bucket has no successful probe).
type overviewDotDTO struct {
	BucketStart string   `json:"bucket_start"`
	Total       int      `json:"total"`
	Failures    int      `json:"failures"`
	P50Ms       *float64 `json:"p50_ms"`
}

// overviewGroupDTO is the health aggregate of one classification group
// (a family or a capability): how its endpoints distribute across statuses
// (disabled counted separately), plus probe-weighted 24h availability and
// mean 24h latency (nil when the group has no probes in the window).
type overviewGroupDTO struct {
	Key             string         `json:"key"`
	EndpointCount   int            `json:"endpoint_count"`
	StatusCounts    map[string]int `json:"status_counts"`
	Availability24h *float64       `json:"availability_24h"`
	AvgLatencyMs    *float64       `json:"avg_latency_ms"`
}

// groupAccumulator aggregates per-endpoint data into group aggregates.
type groupAccumulator struct {
	byKey map[string]*groupBucket
}

// groupBucket is the running aggregate of one group.
type groupBucket struct {
	count        int
	statusCounts map[string]int
	probes       int
	ok           int
	latencySum   int
}

// statusDisabled buckets endpoints that are switched off, separately from
// the status-machine kinds.
const statusDisabled = "disabled"

// newGroupAccumulator creates an empty accumulator.
func newGroupAccumulator() *groupAccumulator {
	return &groupAccumulator{byKey: map[string]*groupBucket{}}
}

// add folds one endpoint (its status entry and 24h samples) into a group.
// Disabled endpoints count toward the endpoint total and the disabled
// bucket, but their samples are excluded from availability and latency: a
// switched-off endpoint is not monitored, so its stale history must not drag
// the group's health metrics.
func (a *groupAccumulator) add(key string, entry overviewEntryDTO, samples []store.ProbeSample) {
	b, ok := a.byKey[key]
	if !ok {
		b = &groupBucket{statusCounts: map[string]int{}}
		a.byKey[key] = b
	}
	b.count++
	statusKey := entry.Status
	if !entry.Enabled {
		statusKey = statusDisabled
		b.statusCounts[statusKey]++
		return
	}
	b.statusCounts[statusKey]++
	for _, s := range samples {
		b.probes++
		b.latencySum += s.LatencyMs
		if s.OK {
			b.ok++
		}
	}
}

// groups returns the finished aggregates ordered by key.
func (a *groupAccumulator) groups() []overviewGroupDTO {
	keys := make([]string, 0, len(a.byKey))
	for k := range a.byKey {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	out := make([]overviewGroupDTO, 0, len(keys))
	for _, k := range keys {
		b := a.byKey[k]
		g := overviewGroupDTO{
			Key:           k,
			EndpointCount: b.count,
			StatusCounts:  b.statusCounts,
		}
		if b.probes > 0 {
			availability := float64(b.ok) / float64(b.probes)
			avg := float64(b.latencySum) / float64(b.probes)
			g.Availability24h = &availability
			g.AvgLatencyMs = &avg
		}
		out = append(out, g)
	}
	return out
}

// handleGetOverview builds the status matrix for every endpoint, plus the
// family and capability group aggregates.
func (s *Server) handleGetOverview(w http.ResponseWriter, r *http.Request) {
	now := s.now().UTC()

	models, err := s.listModelsForRequest(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list models")
		return
	}

	// Batch-load latest eval scores for all models to avoid N+1 queries
	modelIDs := make([]int64, len(models))
	for i, m := range models {
		modelIDs[i] = m.ID
	}
	evalScores, err := s.getEvalScoresForModels(modelIDs)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load eval scores")
		return
	}

	entries := []overviewEntryDTO{}
	families := newGroupAccumulator()
	capabilities := newGroupAccumulator()
	protocols := newGroupAccumulator()
	// Global aggregate reuses the same accumulator semantics (disabled
	// endpoints excluded from the probe metrics) with a single key.
	global := newGroupAccumulator()
	enabledEndpoints := 0
	for _, model := range models {
		endpoints, err := s.db.ListEndpointsByModelID(model.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to list endpoints")
			return
		}
		for _, ep := range endpoints {
			stats, err := s.gatherWindowStats(ep.ID, now)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "failed to build overview")
				return
			}
			entry := buildOverviewEntryFromStats(ep, model, stats, now)
			// Attach eval score from the batch-loaded map
			if score, ok := evalScores[model.ID]; ok {
				entry.EvalScore = score
			}
			entries = append(entries, entry)
			families.add(model.Family, entry, stats.samples24h)
			capabilities.add(model.Capability, entry, stats.samples24h)
			protocols.add(ep.Protocol, entry, stats.samples24h)
			global.add("all", entry, stats.samples24h)
			if ep.Enabled {
				enabledEndpoints++
			}
		}
	}

	dto := overviewDTO{
		GeneratedAt:      now.Format(time.RFC3339),
		Endpoints:        entries,
		ByFamily:         families.groups(),
		ByCapability:     capabilities.groups(),
		ByProtocol:       protocols.groups(),
		EnabledEndpoints: enabledEndpoints,
	}
	if groups := global.groups(); len(groups) > 0 {
		dto.Availability24h = groups[0].Availability24h
	}

	writeData(w, http.StatusOK, dto)
}

// windowStats bundles the probe history the status machine needs for one
// endpoint, shared by the overview matrix and the endpoint detail API.
type windowStats struct {
	total       int
	consecutive int
	latest      *store.Probe
	samples24h  []store.ProbeSample
	baselineP50 float64
	hasBaseline bool
}

// gatherWindowStats collects the status-machine inputs of one endpoint as of
// the given time.
func (s *Server) gatherWindowStats(endpointID int64, now time.Time) (windowStats, error) {
	var stats windowStats
	var err error

	if stats.consecutive, err = s.db.CountConsecutiveFailures(endpointID); err != nil {
		return stats, err
	}
	if stats.total, err = s.db.CountProbes(endpointID); err != nil {
		return stats, err
	}
	if stats.latest, err = s.db.LatestProbe(endpointID); err != nil {
		return stats, err
	}
	if stats.samples24h, err = s.db.ListProbeSamplesSince(endpointID, now.Add(-overviewWindow24h)); err != nil {
		return stats, err
	}
	baselineSamples, err := s.db.ListProbeSamplesSince(endpointID, now.Add(-overviewBaselineSpan))
	if err != nil {
		return stats, err
	}

	// Latency baseline: 7-day P50, skipped when there are too few samples.
	if len(baselineSamples) >= status.MinBaselineSamples {
		stats.baselineP50 = status.Percentile(status.Latencies(toStatusSamples(baselineSamples)), 50)
		stats.hasBaseline = true
	}
	return stats, nil
}

// statusSamples converts the stored 24h samples into status-machine samples.
func (ws windowStats) statusSamples() []status.Sample {
	return toStatusSamples(ws.samples24h)
}

// statusInput assembles the status-machine input from the gathered stats.
func (ws windowStats) statusInput() status.Input {
	lastError := ""
	if ws.latest != nil && !ws.latest.OK && ws.latest.ErrorSummary != nil {
		lastError = *ws.latest.ErrorSummary
	}
	return status.Input{
		TotalProbes:         ws.total,
		ConsecutiveFailures: ws.consecutive,
		LastError:           lastError,
		Samples24h:          ws.statusSamples(),
		BaselineP50Ms:       ws.baselineP50,
		HasBaseline:         ws.hasBaseline,
	}
}

// evaluate runs the status machine over the gathered window stats.
func (ws windowStats) evaluate() status.Result {
	return status.Evaluate(ws.statusInput())
}

// toStatusSamples converts store samples into status-machine samples.
func toStatusSamples(in []store.ProbeSample) []status.Sample {
	out := make([]status.Sample, 0, len(in))
	for _, s := range in {
		out = append(out, status.Sample{OK: s.OK, LatencyMs: s.LatencyMs})
	}
	return out
}

// buildOverviewEntryFromStats assembles the status and 24h statistics of a
// single endpoint from already-gathered window stats.
func buildOverviewEntryFromStats(ep store.Endpoint, model store.Model, stats windowStats, now time.Time) overviewEntryDTO {
	entry := overviewEntryDTO{
		EndpointID:    ep.ID,
		ModelID:       model.ModelID,
		Protocol:      ep.Protocol,
		Enabled:       ep.Enabled,
		Family:        model.Family,
		Capability:    model.Capability,
		DegradeCauses: []string{},
		ScoreReasons:  []string{},
		Dots24h:       buildDots24h(stats.samples24h, now),
	}

	if stats.latest != nil {
		ts := stats.latest.CreatedAt.UTC().Format(time.RFC3339)
		entry.LastProbeAt = &ts
	}

	stats24h := stats.statusSamples()

	// 24h summary fields: null when the window has no data.
	if rate, ok := status.SuccessRate(stats24h); ok {
		entry.SuccessRate24h = &rate
	}
	if len(stats24h) > 0 {
		latencies := status.Latencies(stats24h)
		p50 := status.Percentile(latencies, 50)
		p95 := status.Percentile(latencies, 95)
		entry.P50Ms = &p50
		entry.P95Ms = &p95
	}

	result := stats.evaluate()
	entry.Status = string(result.Kind)
	entry.StatusReason = result.Reason
	for _, c := range result.Causes {
		entry.DegradeCauses = append(entry.DegradeCauses, string(c))
	}

	// Expose the same baseline the status machine used — a recomputed value
	// could disagree with the degradation verdict on the same card.
	if stats.hasBaseline {
		entry.BaselineP50Ms = &stats.baselineP50
	}

	// Deterministic score over the same inputs; null when never probed.
	score := status.Score(stats.statusInput())
	if score.HasScore {
		entry.Score = &score.Value
	}
	entry.ScoreReasons = score.Reasons

	return entry
}

// buildDots24h buckets the 24h probe samples into exactly 24 hour-aligned
// buckets ending at the current hour. Empty buckets stay present with zero
// counts; samples older than the first bucket are dropped. Each bucket's
// p50_ms is the P50 of its successful probes' latencies (collected in the
// same bucketing pass — no second bucketing), null when the bucket has no
// successful probe.
func buildDots24h(samples []store.ProbeSample, now time.Time) []overviewDotDTO {
	last := now.UTC().Truncate(time.Hour)
	first := last.Add(-23 * time.Hour)

	dots := make([]overviewDotDTO, 24)
	okLatencies := make([][]int, 24)
	index := make(map[time.Time]int, 24)
	for i := range dots {
		start := first.Add(time.Duration(i) * time.Hour)
		dots[i] = overviewDotDTO{BucketStart: start.Format(time.RFC3339)}
		index[start] = i
	}
	for _, s := range samples {
		bucket := s.CreatedAt.UTC().Truncate(time.Hour)
		i, ok := index[bucket]
		if !ok {
			continue
		}
		dots[i].Total++
		if !s.OK {
			dots[i].Failures++
		} else {
			okLatencies[i] = append(okLatencies[i], s.LatencyMs)
		}
	}
	for i := range dots {
		if len(okLatencies[i]) > 0 {
			p50 := status.Percentile(okLatencies[i], 50)
			dots[i].P50Ms = &p50
		}
	}
	return dots
}

// getEvalScoresForModels batch-loads the latest evaluation total scores for
// all provided models. Returns a map of model_id → total_score; models without
// any eval results are absent from the map. This avoids N+1 queries when
// building the overview response (ticket 60.2).
func (s *Server) getEvalScoresForModels(modelIDs []int64) (map[int64]*float64, error) {
	campaigns, err := s.db.GetLatestCampaignsForModels(modelIDs)
	if err != nil {
		return nil, err
	}

	if len(campaigns) == 0 {
		return map[int64]*float64{}, nil
	}

	// For each campaign, compute its scores using the same logic as the
	// report endpoint. We need to process each campaign separately because
	// different campaigns may have different suite versions and nadirs.
	result := make(map[int64]*float64)

	for modelID, campaign := range campaigns {
		// Get the runs for this campaign to extract suite metadata
		runs, err := s.db.ListEvalRunsByCampaign(campaign.ID)
		if err != nil {
			return nil, err
		}

		// Build the suite list with versions from runs
		suites, err := s.campaignSuites(runs)
		if err != nil {
			return nil, err
		}

		if len(suites) == 0 {
			// Campaign exists but has no runs or suites
			continue
		}

		// Get weights configuration
		configured, err := s.db.GetSuiteWeights()
		if err != nil {
			return nil, err
		}
		weights := effectiveWeights(suites, configured)

		// Load scores for this campaign
		scores, err := s.db.ListCampaignSuiteScores(campaign.ID)
		if err != nil {
			return nil, err
		}

		// Build nadirs map from runs (same as campaign report)
		nadirs := nadirBySuiteKey(suites, runs)

		// Compute report rows with total scores
		rows := buildReportRows(scores, weights, nadirs)

		// Find this model's total score
		for _, row := range rows {
			if row.ModelDBID == modelID && row.TotalScore != nil {
				result[modelID] = row.TotalScore
				break
			}
		}
	}

	return result, nil
}
