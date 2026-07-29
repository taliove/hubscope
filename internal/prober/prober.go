package prober

import (
	"context"
	"log/slog"

	"github.com/taliove/hubscope/internal/hubclient"
	"github.com/taliove/hubscope/internal/store"
)

// Prober runs probe rounds against endpoints and persists the results.
type Prober struct {
	db     *store.DB
	client *hubclient.Client

	// AfterRound, when set, is invoked synchronously at the end of every
	// round (manual or scheduled) with the persisted results. The alert
	// evaluator hooks in here; it must never panic and handles its own
	// errors internally.
	AfterRound func(ctx context.Context, endpointID int64, results []store.Probe)
}

// New creates a Prober backed by the given store and hub client.
func New(db *store.DB, client *hubclient.Client) *Prober {
	return &Prober{db: db, client: client}
}

// RunRound executes one probe round for a single endpoint. Chat protocols run
// a non-streaming request followed by a streaming request (serially), image
// protocols run a single call — they have no streaming mode or TTFT concept
// (spec 0014). All probe records are persisted and returned in order.
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

	var probes []store.Probe
	if hubclient.IsImageProtocol(endpoint.Protocol) {
		probes = []store.Probe{p.probeAndStore(ctx, hub, model, endpoint, false, p.imageParamsFor(model))}
	} else {
		// Non-streaming first, then streaming.
		nonStream := p.probeAndStore(ctx, hub, model, endpoint, false, nil)
		stream := p.probeAndStore(ctx, hub, model, endpoint, true, nil)
		probes = []store.Probe{nonStream, stream}
	}
	if p.AfterRound != nil {
		// A misbehaving hook must never take down probing.
		func() {
			defer func() {
				if r := recover(); r != nil {
					slog.Error("prober: AfterRound hook panicked", "endpoint_id", endpointID, "panic", r)
				}
			}()
			p.AfterRound(ctx, endpointID, probes)
		}()
	}
	return probes, nil
}

// imageParamsFor resolves the rule-merged extra probe parameters for an
// image-capable model via the single resolution entry (store.ImageParamsFor,
// GH #33). A rules-table hiccup must never break probing: on error the round
// degrades to the minimal request body and the failure is logged.
func (p *Prober) imageParamsFor(model *store.Model) map[string]string {
	params, err := p.db.ImageParamsFor(model.ModelID)
	if err != nil {
		slog.Warn("prober: image param rules unavailable, probing with minimal body",
			"model", model.ModelID, "error", err)
		return nil
	}
	return params
}

// probeAndStore runs one probe and writes the result to the store. imageParams
// is non-nil only for image protocols (chat probes take the minimal fixed
// chat body).
func (p *Prober) probeAndStore(ctx context.Context, hub *store.Hub, model *store.Model, endpoint *store.Endpoint, streaming bool, imageParams map[string]string) store.Probe {
	result := p.client.Probe(ctx, hub.BaseURL, hub.Token, endpoint.Protocol, model.ModelID, streaming, imageParams)

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
