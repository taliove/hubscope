package server

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/taliove/hubscope/internal/store"
)

// Series query parameter bounds from api-contract.md.
const (
	defaultSeriesHours = 24
	maxSeriesHours     = 2160 // 90 days
)

// endpointDetailDTO is the response of GET /api/endpoints/{id}: the Endpoint
// fields plus model string, hub name, and the status-machine result.
type endpointDetailDTO struct {
	endpointDTO
	ModelIDStr   string `json:"model_id_str"`
	HubName      string `json:"hub_name"`
	Status       string `json:"status"`
	StatusReason string `json:"status_reason"`
}

// seriesBucketDTO is one hourly bucket of the series API.
type seriesBucketDTO struct {
	BucketStart string   `json:"bucket_start"`
	Total       int      `json:"total"`
	Failures    int      `json:"failures"`
	P50Ms       *float64 `json:"p50_ms"`
	P95Ms       *float64 `json:"p95_ms"`
	AvgTTFTMs   *float64 `json:"avg_ttft_ms"`
}

// handleGetEndpointDetail handles GET /api/endpoints/{id}.
func (s *Server) handleGetEndpointDetail(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid endpoint id")
		return
	}

	endpoint, err := s.db.GetEndpoint(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "endpoint not found")
		return
	}
	model, err := s.db.GetModel(endpoint.ModelID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load model")
		return
	}
	hub, err := s.db.GetHub(model.HubID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load hub")
		return
	}

	stats, err := s.gatherWindowStats(id, s.now().UTC())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to evaluate status")
		return
	}
	result := stats.evaluate()

	writeData(w, http.StatusOK, endpointDetailDTO{
		endpointDTO:  toEndpointDTO(*endpoint),
		ModelIDStr:   model.ModelID,
		HubName:      hub.Name,
		Status:       string(result.Kind),
		StatusReason: result.Reason,
	})
}

// handleGetEndpointSeries handles GET /api/endpoints/{id}/series.
func (s *Server) handleGetEndpointSeries(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid endpoint id")
		return
	}
	if _, err := s.db.GetEndpoint(id); err != nil {
		writeError(w, http.StatusNotFound, "endpoint not found")
		return
	}

	hours, err := parseSeriesHours(r.URL.Query().Get("hours"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	mode, err := parseSeriesMode(r.URL.Query().Get("streaming"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	since := s.now().UTC().Add(-time.Duration(hours) * time.Hour)
	buckets, err := s.db.GetSeries(id, since, mode)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load series")
		return
	}

	dtos := make([]seriesBucketDTO, 0, len(buckets))
	for _, b := range buckets {
		dtos = append(dtos, seriesBucketDTO{
			BucketStart: b.BucketStart.UTC().Format(time.RFC3339),
			Total:       b.Total,
			Failures:    b.Failures,
			P50Ms:       b.P50Ms,
			P95Ms:       b.P95Ms,
			AvgTTFTMs:   b.AvgTTFTMs,
		})
	}
	writeData(w, http.StatusOK, dtos)
}

// parseSeriesHours validates the hours query parameter: an integer in
// [1, 2160], defaulting to 24 when absent.
func parseSeriesHours(raw string) (int, error) {
	if raw == "" {
		return defaultSeriesHours, nil
	}
	hours, err := strconv.Atoi(raw)
	if err != nil || hours < 1 || hours > maxSeriesHours {
		return 0, fmt.Errorf("hours must be an integer between 1 and %d", maxSeriesHours)
	}
	return hours, nil
}

// parseSeriesMode validates the streaming query parameter: all, streaming, or
// non_streaming, defaulting to all when absent.
func parseSeriesMode(raw string) (store.SeriesMode, error) {
	switch raw {
	case "", "all":
		return store.SeriesAll, nil
	case "streaming":
		return store.SeriesStreaming, nil
	case "non_streaming":
		return store.SeriesNonStreaming, nil
	default:
		return store.SeriesAll, fmt.Errorf("streaming must be one of: all, streaming, non_streaming")
	}
}
