// Package discovery synchronizes the registered model list with each hub's
// /v1/models listing: new models are registered and probed per protocol,
// vanished discovered models are retired (history kept), reappearing models
// are reactivated, and manual models are never retired.
package discovery

import (
	"context"
	"errors"
	"log"
	"strings"
	"sync"

	"git.github.net/taliove2009/ai-hub-checker/internal/hubclient"
	"git.github.net/taliove2009/ai-hub-checker/internal/store"
)

// NonChatKeywords marks model IDs that are not conversational chat models
// (image generation, embeddings, speech, moderation, reranking, ...). A model
// whose lowercase ID contains any of these is tagged capability='non_chat'
// and excluded from evaluation candidates.
var NonChatKeywords = []string{
	"image",
	"embedding",
	"tts",
	"dall",
	"whisper",
	"moderation",
	"audio",
	"rerank",
}

// protocols lists both hub API protocols in canonical endpoint order.
var protocols = []string{"anthropic", "openai"}

// Stats summarizes one sync run across all hubs.
type Stats struct {
	Added            int `json:"added"`
	Retired          int `json:"retired"`
	EndpointsCreated int `json:"endpoints_created"`
}

// ErrSyncInProgress is returned when a sync is already running for the hub.
var ErrSyncInProgress = errors.New("discovery: sync already in progress for hub")

// Syncer reconciles the store with hub model listings.
type Syncer struct {
	db     *store.DB
	client *hubclient.Client

	mu       sync.Mutex
	inflight map[int64]bool
}

// New creates a Syncer over the given store and hub client.
func New(db *store.DB, client *hubclient.Client) *Syncer {
	return &Syncer{db: db, client: client, inflight: make(map[int64]bool)}
}

// ClassifyCapability returns 'non_chat' when the model ID contains a known
// non-conversational keyword, otherwise 'chat'.
func ClassifyCapability(modelID string) string {
	lower := strings.ToLower(modelID)
	for _, kw := range NonChatKeywords {
		if strings.Contains(lower, kw) {
			return "non_chat"
		}
	}
	return "chat"
}

// Sync runs one full discovery pass over all hubs and returns aggregated
// stats. A hub that fails to list models is logged and skipped so one bad
// hub never blocks the others; a hub already syncing (e.g. triggered via the
// API) is skipped too.
func (s *Syncer) Sync(ctx context.Context) (Stats, error) {
	var total Stats
	hubs, err := s.db.ListHubs()
	if err != nil {
		return total, err
	}
	for _, hub := range hubs {
		if !s.acquire(hub.ID) {
			log.Printf("discovery: hub %d (%s) sync already in progress, skipping", hub.ID, hub.Name)
			continue
		}
		stats, err := s.syncOne(ctx, hub)
		if err != nil {
			log.Printf("discovery: sync hub %d (%s): %v", hub.ID, hub.Name, err)
			continue
		}
		total.Added += stats.Added
		total.Retired += stats.Retired
		total.EndpointsCreated += stats.EndpointsCreated
	}
	return total, nil
}

// syncOne runs one guarded hub sync inside the full-sync loop, always
// releasing the guard — unlike SyncHub/StartSync the loop has no defer, so
// this wrapper keeps a panicking sync from wedging the hub's guard.
func (s *Syncer) syncOne(ctx context.Context, hub store.Hub) (stats Stats, err error) {
	defer s.release(hub.ID)
	if err := s.db.SetHubSyncing(hub.ID); err != nil {
		return Stats{}, err
	}
	return s.syncMarked(ctx, hub)
}

// StartSync launches an asynchronous sync for one hub and returns immediately.
// The in-flight guard and the persisted 'syncing' mark are both taken before
// returning, so a concurrent trigger sees ErrSyncInProgress and any read sees
// the syncing state deterministically. Failures afterwards surface only
// through the hub's persisted sync status and the log.
func (s *Syncer) StartSync(hubID int64) error {
	if !s.acquire(hubID) {
		return ErrSyncInProgress
	}
	if err := s.db.SetHubSyncing(hubID); err != nil {
		s.release(hubID)
		return err
	}
	go func() {
		defer s.release(hubID)
		hub, err := s.db.GetHub(hubID)
		if err != nil {
			log.Printf("discovery: async sync hub %d: load hub: %v", hubID, err)
			return
		}
		if _, err := s.syncMarked(context.Background(), *hub); err != nil {
			log.Printf("discovery: async sync hub %d: %v", hubID, err)
		}
	}()
	return nil
}

// acquire takes the per-hub in-flight guard; false means it was already held.
func (s *Syncer) acquire(hubID int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.inflight[hubID] {
		return false
	}
	s.inflight[hubID] = true
	return true
}

// release drops the per-hub in-flight guard.
func (s *Syncer) release(hubID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.inflight, hubID)
}

// syncMarked syncs one hub whose 'syncing' mark is already persisted, then
// records the outcome. The caller must hold the hub's in-flight guard.
func (s *Syncer) syncMarked(ctx context.Context, hub store.Hub) (Stats, error) {
	stats, syncErr := s.syncHub(ctx, hub)
	if syncErr != nil {
		msg := syncErr.Error()
		if err := s.db.SetHubSyncResult(hub.ID, &msg); err != nil {
			log.Printf("discovery: persist sync failure for hub %d: %v", hub.ID, err)
		}
		return stats, syncErr
	}
	if err := s.db.SetHubSyncResult(hub.ID, nil); err != nil {
		return stats, err
	}
	log.Printf("discovery: hub %d (%s) synced: added=%d retired=%d endpoints_created=%d",
		hub.ID, hub.Name, stats.Added, stats.Retired, stats.EndpointsCreated)
	return stats, nil
}

// syncHub reconciles a single hub: register new models, reactivate returning
// ones, and retire discovered models missing from the listing.
func (s *Syncer) syncHub(ctx context.Context, hub store.Hub) (Stats, error) {
	var stats Stats

	ids, err := s.client.ListModels(ctx, hub.BaseURL, hub.Token)
	if err != nil {
		return stats, err
	}

	for _, id := range ids {
		model, created, err := s.db.CreateDiscoveredModel(hub.ID, id, ClassifyCapability(id))
		if err != nil {
			return stats, err
		}
		if !created {
			continue
		}
		stats.Added++
		if err := s.probeAndCreateEndpoints(ctx, hub, model); err != nil {
			return stats, err
		}
		stats.EndpointsCreated += len(protocols)
	}

	retired, err := s.db.MarkRetiredMissing(hub.ID, ids)
	if err != nil {
		return stats, err
	}
	stats.Retired += retired
	return stats, nil
}

// probeAndCreateEndpoints fires one minimal non-streaming request per
// protocol at a newly discovered model and creates one endpoint per
// protocol: enabled where the probe succeeded, disabled where it failed —
// both are created so the UI shows which protocol is unreachable. The trial
// probes are persisted as probe records so the failure reason (HTTP status,
// upstream error) stays visible through the probes API and detail pages.
func (s *Syncer) probeAndCreateEndpoints(ctx context.Context, hub store.Hub, model *store.Model) error {
	for _, protocol := range protocols {
		result := s.client.Probe(ctx, hub.BaseURL, hub.Token, protocol, model.ModelID, false)
		endpoint, err := s.db.CreateEndpoint(model.ID, protocol, result.OK)
		if err != nil {
			return err
		}
		_, err = s.db.CreateProbe(store.Probe{
			EndpointID:   endpoint.ID,
			Streaming:    false,
			OK:           result.OK,
			HTTPStatus:   result.HTTPStatus,
			ErrorSummary: result.ErrorSummary,
			LatencyMs:    result.LatencyMs,
			TTFTMs:       result.TTFTMs,
			InputTokens:  result.InputTokens,
			OutputTokens: result.OutputTokens,
		})
		if err != nil {
			return err
		}
	}
	return nil
}
