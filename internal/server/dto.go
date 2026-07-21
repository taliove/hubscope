package server

import (
	"time"

	"git.github.net/taliove2009/ai-hub-checker/internal/store"
)

// hubDTO is the API representation of a Hub. It never carries the raw token.
type hubDTO struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	BaseURL   string `json:"base_url"`
	TokenHint string `json:"token_hint"`
	CreatedAt string `json:"created_at"`
}

// endpointDTO is the API representation of an Endpoint.
type endpointDTO struct {
	ID       int64  `json:"id"`
	ModelID  int64  `json:"model_id"`
	Protocol string `json:"protocol"`
	Enabled  bool   `json:"enabled"`
}

// modelDTO is the API representation of a Model with its endpoints.
type modelDTO struct {
	ID         int64         `json:"id"`
	HubID      int64         `json:"hub_id"`
	ModelID    string        `json:"model_id"`
	Origin     string        `json:"origin"`
	Status     string        `json:"status"`
	Capability string        `json:"capability"`
	Endpoints  []endpointDTO `json:"endpoints"`
}

// probeDTO is the API representation of a ProbeRecord.
type probeDTO struct {
	ID           int64   `json:"id"`
	EndpointID   int64   `json:"endpoint_id"`
	Streaming    bool    `json:"streaming"`
	OK           bool    `json:"ok"`
	HTTPStatus   int     `json:"http_status"`
	ErrorSummary *string `json:"error_summary"`
	LatencyMs    int     `json:"latency_ms"`
	TTFTMs       *int    `json:"ttft_ms"`
	InputTokens  *int    `json:"input_tokens"`
	OutputTokens *int    `json:"output_tokens"`
	CreatedAt    string  `json:"created_at"`
}

// maskToken returns the last 4 characters of a token prefixed with an ellipsis.
// A token shorter than 4 characters yields just the ellipsis.
func maskToken(token string) string {
	runes := []rune(token)
	if len(runes) < 4 {
		return "…"
	}
	return "…" + string(runes[len(runes)-4:])
}

// toHubDTO maps a store.Hub to its API representation.
func toHubDTO(h store.Hub) hubDTO {
	return hubDTO{
		ID:        h.ID,
		Name:      h.Name,
		BaseURL:   h.BaseURL,
		TokenHint: maskToken(h.Token),
		CreatedAt: h.CreatedAt.Format(time.RFC3339),
	}
}

// toEndpointDTO maps a store.Endpoint to its API representation.
func toEndpointDTO(e store.Endpoint) endpointDTO {
	return endpointDTO{
		ID:       e.ID,
		ModelID:  e.ModelID,
		Protocol: e.Protocol,
		Enabled:  e.Enabled,
	}
}

// toModelDTO maps a store.Model plus its endpoints to the API representation.
func toModelDTO(m store.Model, endpoints []store.Endpoint) modelDTO {
	eps := make([]endpointDTO, 0, len(endpoints))
	for _, e := range endpoints {
		eps = append(eps, toEndpointDTO(e))
	}
	return modelDTO{
		ID:         m.ID,
		HubID:      m.HubID,
		ModelID:    m.ModelID,
		Origin:     m.Origin,
		Status:     m.Status,
		Capability: m.Capability,
		Endpoints:  eps,
	}
}

// toProbeDTO maps a store.Probe to the API representation.
func toProbeDTO(p store.Probe) probeDTO {
	return probeDTO{
		ID:           p.ID,
		EndpointID:   p.EndpointID,
		Streaming:    p.Streaming,
		OK:           p.OK,
		HTTPStatus:   p.HTTPStatus,
		ErrorSummary: p.ErrorSummary,
		LatencyMs:    p.LatencyMs,
		TTFTMs:       p.TTFTMs,
		InputTokens:  p.InputTokens,
		OutputTokens: p.OutputTokens,
		CreatedAt:    p.CreatedAt.Format(time.RFC3339),
	}
}
