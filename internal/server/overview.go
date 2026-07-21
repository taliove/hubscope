package server

import (
	"net/http"
	"time"

	"git.github.net/taliove2009/ai-hub-checker/internal/status"
	"git.github.net/taliove2009/ai-hub-checker/internal/store"
)

// overviewWindows defines the lookback windows used by the status machine.
const (
	overviewWindow24h    = 24 * time.Hour
	overviewBaselineSpan = 7 * 24 * time.Hour
)

// overviewDTO is the response body of GET /api/overview.
type overviewDTO struct {
	GeneratedAt string             `json:"generated_at"`
	Endpoints   []overviewEntryDTO `json:"endpoints"`
}

// overviewEntryDTO is the per-endpoint status summary. Field names follow
// the api-contract.md Overview section exactly.
type overviewEntryDTO struct {
	EndpointID     int64    `json:"endpoint_id"`
	ModelID        string   `json:"model_id"`
	Protocol       string   `json:"protocol"`
	Enabled        bool     `json:"enabled"`
	Status         string   `json:"status"`
	StatusReason   string   `json:"status_reason"`
	SuccessRate24h *float64 `json:"success_rate_24h"`
	P50Ms          *float64 `json:"p50_ms"`
	P95Ms          *float64 `json:"p95_ms"`
	LastProbeAt    *string  `json:"last_probe_at"`
}

// handleGetOverview builds the status matrix for every endpoint.
func (s *Server) handleGetOverview(w http.ResponseWriter, r *http.Request) {
	now := s.now().UTC()

	models, err := s.db.ListModels()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list models")
		return
	}

	entries := []overviewEntryDTO{}
	for _, model := range models {
		endpoints, err := s.db.ListEndpointsByModelID(model.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to list endpoints")
			return
		}
		for _, ep := range endpoints {
			entry, err := s.buildOverviewEntry(ep, model.ModelID, now)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "failed to build overview")
				return
			}
			entries = append(entries, entry)
		}
	}

	writeData(w, http.StatusOK, overviewDTO{
		GeneratedAt: now.Format(time.RFC3339),
		Endpoints:   entries,
	})
}

// buildOverviewEntry assembles the status and 24h statistics of a single
// endpoint as of the given time.
func (s *Server) buildOverviewEntry(ep store.Endpoint, modelID string, now time.Time) (overviewEntryDTO, error) {
	entry := overviewEntryDTO{
		EndpointID: ep.ID,
		ModelID:    modelID,
		Protocol:   ep.Protocol,
		Enabled:    ep.Enabled,
	}

	consecutive, err := s.db.CountConsecutiveFailures(ep.ID)
	if err != nil {
		return entry, err
	}
	total, err := s.db.CountProbes(ep.ID)
	if err != nil {
		return entry, err
	}
	latest, err := s.db.LatestProbe(ep.ID)
	if err != nil {
		return entry, err
	}
	samples24h, err := s.db.ListProbeSamplesSince(ep.ID, now.Add(-overviewWindow24h))
	if err != nil {
		return entry, err
	}
	baselineSamples, err := s.db.ListProbeSamplesSince(ep.ID, now.Add(-overviewBaselineSpan))
	if err != nil {
		return entry, err
	}

	if latest != nil {
		ts := latest.CreatedAt.UTC().Format(time.RFC3339)
		entry.LastProbeAt = &ts
	}

	// Convert store samples into status-machine samples.
	toSamples := func(in []store.ProbeSample) []status.Sample {
		out := make([]status.Sample, 0, len(in))
		for _, s := range in {
			out = append(out, status.Sample{OK: s.OK, LatencyMs: s.LatencyMs})
		}
		return out
	}
	stats24h := toSamples(samples24h)

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

	// Latency baseline: 7-day P50, skipped when there are too few samples.
	var baselineP50 float64
	hasBaseline := false
	if len(baselineSamples) >= status.MinBaselineSamples {
		baselineP50 = status.Percentile(status.Latencies(toSamples(baselineSamples)), 50)
		hasBaseline = true
	}

	lastError := ""
	if latest != nil && !latest.OK && latest.ErrorSummary != nil {
		lastError = *latest.ErrorSummary
	}

	result := status.Evaluate(status.Input{
		TotalProbes:         total,
		ConsecutiveFailures: consecutive,
		LastError:           lastError,
		Samples24h:          stats24h,
		BaselineP50Ms:       baselineP50,
		HasBaseline:         hasBaseline,
	})
	entry.Status = string(result.Kind)
	entry.StatusReason = result.Reason

	return entry, nil
}
