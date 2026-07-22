// Package discovery synchronizes the registered model list with each hub's
// /v1/models listing: new models are registered and probed per protocol,
// vanished discovered models are retired (history kept), reappearing models
// are reactivated, and manual models are never retired. Classification
// (capability + family) comes from the classifier package's rule set.
package discovery

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/taliove2009/hubscope/internal/classifier"
	"github.com/taliove2009/hubscope/internal/hubclient"
	"github.com/taliove2009/hubscope/internal/store"
)

// protocols lists both hub API protocols in canonical endpoint order.
var protocols = []string{"anthropic", "openai"}

// Stats summarizes one sync run across all hubs.
type Stats struct {
	Added            int `json:"added"`
	Updated          int `json:"updated"`
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

// syncHub reconciles a single hub: register new models, reactivate returning
// ones, and retire discovered models missing from the listing.
func (s *Syncer) syncHub(ctx context.Context, hub store.Hub) (Stats, error) {
	var stats Stats

	ids, err := s.client.ListModels(ctx, hub.BaseURL, hub.Token)
	if err != nil {
		return stats, err
	}

	rules, err := s.db.ListClassificationRules()
	if err != nil {
		return stats, err
	}

	for _, id := range ids {
		capability, family := classifier.Classify(id, rules)
		model, created, err := s.db.CreateDiscoveredModel(hub.ID, id, capability, family)
		if err != nil {
			return stats, err
		}
		if created {
			stats.Added++
		} else {
			stats.Updated++
		}
		// New models trial both protocols; existing ones only trial the
		// protocols they still lack, so a protocol that becomes available
		// later gets backfilled on the next sync.
		endpoints, err := s.trialAndCreateEndpoints(ctx, hub, model)
		if err != nil {
			return stats, err
		}
		stats.EndpointsCreated += endpoints
	}

	retired, err := s.db.MarkRetiredMissing(hub.ID, ids)
	if err != nil {
		return stats, err
	}
	stats.Retired += retired
	return stats, nil
}

// Sync runs one full discovery pass over all hubs and returns aggregated
// stats. A hub that fails to list models is logged and skipped so one bad
// hub never blocks the others; a hub already syncing (e.g. triggered via the
// API) is skipped too. Each synced hub registers a task in the task center
// with the given source (manual for API triggers, scheduled for the
// periodic loop).
func (s *Syncer) Sync(ctx context.Context, source string) (Stats, error) {
	var total Stats
	hubs, err := s.db.ListHubs()
	if err != nil {
		return total, err
	}
	for _, hub := range hubs {
		if !s.acquire(hub.ID) {
			slog.Debug("discovery: sync already in progress, skipping", "hub_id", hub.ID, "hub", hub.Name)
			continue
		}
		stats, err := s.syncOne(ctx, hub, source)
		if err != nil {
			slog.Error("discovery: sync hub failed", "hub_id", hub.ID, "hub", hub.Name, "error", err)
			continue
		}
		total.Added += stats.Added
		total.Updated += stats.Updated
		total.Retired += stats.Retired
		total.EndpointsCreated += stats.EndpointsCreated
	}
	return total, nil
}

// syncOne runs one guarded hub sync inside the full-sync loop, always
// releasing the guard — unlike SyncHub/StartSync the loop has no defer, so
// this wrapper keeps a panicking sync from wedging the hub's guard.
func (s *Syncer) syncOne(ctx context.Context, hub store.Hub, source string) (stats Stats, err error) {
	defer s.release(hub.ID)
	if err := s.db.SetHubSyncing(hub.ID); err != nil {
		return Stats{}, err
	}
	return s.syncMarked(ctx, hub, source)
}

// StartSync launches an asynchronous sync for one hub and returns immediately.
// The in-flight guard and the persisted 'syncing' mark are both taken before
// returning, so a concurrent trigger sees ErrSyncInProgress and any read sees
// the syncing state deterministically. Failures afterwards surface only
// through the hub's persisted sync status, the task center and the log.
func (s *Syncer) StartSync(hubID int64, source string) error {
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
			slog.Error("discovery: async sync: load hub", "hub_id", hubID, "error", err)
			return
		}
		if _, err := s.syncMarked(context.Background(), *hub, source); err != nil {
			slog.Error("discovery: async sync failed", "hub_id", hubID, "error", err)
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
// records the outcome. The caller must hold the hub's in-flight guard. Every
// sync registers a discovery_sync task so the task center shows what changed;
// tracking failures never break the sync itself.
func (s *Syncer) syncMarked(ctx context.Context, hub store.Hub, source string) (Stats, error) {
	tracker := s.db.BeginTask(store.TaskTypeDiscoverySync, source, store.TaskEntityHub, hub.ID,
		fmt.Sprintf("discovery sync started: hub=%q", hub.Name))

	stats, syncErr := s.syncHub(ctx, hub)
	if syncErr != nil {
		msg := syncErr.Error()
		if err := s.db.SetHubSyncResult(hub.ID, &msg); err != nil {
			slog.Error("discovery: persist sync failure", "hub_id", hub.ID, "error", err)
		}
		tracker.Fail(fmt.Sprintf("discovery sync failed: hub=%q error=%s", hub.Name, msg))
		return stats, syncErr
	}
	if err := s.db.SetHubSyncResult(hub.ID, nil); err != nil {
		tracker.Fail(fmt.Sprintf("discovery sync failed: hub=%q error=%s", hub.Name, err.Error()))
		return stats, err
	}
	tracker.Succeed(fmt.Sprintf(
		"discovery sync finished: hub=%q added=%d updated=%d retired=%d endpoints_created=%d",
		hub.Name, stats.Added, stats.Updated, stats.Retired, stats.EndpointsCreated))
	slog.Info("discovery: hub synced",
		"hub_id", hub.ID, "hub", hub.Name,
		"added", stats.Added, "updated", stats.Updated, "retired", stats.Retired, "endpoints_created", stats.EndpointsCreated)
	return stats, nil
}

// trialAndCreateEndpoints trial-probes the model on every protocol it does
// not yet have an endpoint for and creates an enabled endpoint per protocol
// that answered. Failed trials create nothing — they are logged, so no
// permanently-disabled placeholder endpoints accumulate (ticket 17).
func (s *Syncer) trialAndCreateEndpoints(ctx context.Context, hub store.Hub, model *store.Model) (int, error) {
	existing, err := s.db.ListEndpointsByModelID(model.ID)
	if err != nil {
		return 0, err
	}
	have := make(map[string]bool, len(existing))
	for _, ep := range existing {
		have[ep.Protocol] = true
	}

	created := 0
	for _, protocol := range protocols {
		if have[protocol] {
			continue
		}
		result := s.client.Probe(ctx, hub.BaseURL, hub.Token, protocol, model.ModelID, false)
		if !result.OK {
			slog.Debug("discovery: protocol trial failed",
				"hub_id", hub.ID, "model", model.ModelID, "protocol", protocol,
				"http_status", result.HTTPStatus, "error", errSummary(result.ErrorSummary))
			continue
		}
		if _, isNew, err := s.db.CreateEndpoint(model.ID, protocol, true); err != nil {
			return created, err
		} else if isNew {
			created++
		}
	}
	return created, nil
}

// errSummary dereferences a probe error summary for logging.
func errSummary(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
