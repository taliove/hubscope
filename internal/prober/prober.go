package prober

import (
	"context"

	"git.github.net/taliove2009/ai-hub-checker/internal/hubclient"
	"git.github.net/taliove2009/ai-hub-checker/internal/store"
)

// Prober runs probe rounds against endpoints and persists the results.
type Prober struct {
	db     *store.DB
	client *hubclient.Client
}

// New creates a Prober backed by the given store and hub client.
func New(db *store.DB, client *hubclient.Client) *Prober {
	return &Prober{db: db, client: client}
}

// RunRound executes one probe round for a single endpoint: a non-streaming
// request followed by a streaming request (serially). Both probe records are
// persisted and returned in order.
func (p *Prober) RunRound(ctx context.Context, endpointID int64) ([]store.Probe, error) {
	endpoint, err := p.db.GetEndpoint(endpointID)
	if err != nil {
		return nil, err
	}

	model, err := p.db.GetModel(endpoint.ModelID)
	if err != nil {
		return nil, err
	}

	hub, err := p.db.GetHub(model.HubID)
	if err != nil {
		return nil, err
	}

	// Non-streaming first, then streaming.
	nonStream := p.probeAndStore(ctx, hub, model, endpoint, false)
	stream := p.probeAndStore(ctx, hub, model, endpoint, true)

	return []store.Probe{nonStream, stream}, nil
}

// probeAndStore runs one probe and writes the result to the store.
func (p *Prober) probeAndStore(ctx context.Context, hub *store.Hub, model *store.Model, endpoint *store.Endpoint, streaming bool) store.Probe {
	result := p.client.Probe(ctx, hub.BaseURL, hub.Token, endpoint.Protocol, model.ModelID, streaming)

	probe := store.Probe{
		EndpointID:   endpoint.ID,
		Streaming:    streaming,
		OK:           result.OK,
		HTTPStatus:   result.HTTPStatus,
		ErrorSummary: result.ErrorSummary,
		LatencyMs:    result.LatencyMs,
		TTFTMs:       result.TTFTMs,
		InputTokens:  result.InputTokens,
		OutputTokens: result.OutputTokens,
	}

	// If persistence fails the in-memory record is still returned so the
	// caller can surface the probe outcome.
	stored, err := p.db.CreateProbe(probe)
	if err != nil {
		return probe
	}
	return stored
}
