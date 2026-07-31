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
	"strings"
	"sync"

	"github.com/taliove/hubscope/internal/classifier"
	"github.com/taliove/hubscope/internal/hubclient"
	"github.com/taliove/hubscope/internal/store"
)

// protocols lists the chat hub API protocols in canonical endpoint order.
// Every model is trialed on them; image-capable models additionally trial
// the image protocol (see trialProtocolsFor).
var protocols = []string{"anthropic", "openai"}

// trialProtocolsFor returns the protocols a model is trial-probed on: chat
// protocols for every model, plus the image protocols for image-capable
// models only (spec 0014 / ADR 0012) — a wrong image trial would burn money
// per call and produce upstream noise, so the gate stays strict. The image
// trials run after the chat trials: a fast-failing chat path costs nothing,
// and image trials only happen for models that classify as image anyway.
// Generation comes before edit within the image family (GH #32): the two
// are independent trials with independent outcomes.
func trialProtocolsFor(capability string) []string {
	list := append([]string(nil), protocols...)
	if capability == "image" {
		list = append(list, hubclient.ProtocolImagesGeneration, hubclient.ProtocolImagesEdit)
	}
	return list
}

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

	// AfterSync, when set, is invoked once after every hub sync (syncMarked),
	// success or failure — a failed sync may already have written models and
	// endpoints mid-loop, so listeners (the overview snapshot's invalidation,
	// spec 0015 decision 3) must fire either way. It runs synchronously on
	// the sync goroutine; keep it cheap.
	AfterSync func(hubID int64)

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

	// Query models about to be retired before marking them (GH #98, spec 0018
	// T4): the aggregated retirement alert needs the model_id list.
	retiredBefore, err := s.db.ListRetiredModels(hub.ID)
	if err != nil {
		return stats, err
	}

	retired, err := s.db.MarkRetiredMissing(hub.ID, ids)
	if err != nil {
		return stats, err
	}
	stats.Retired += retired

	// Emit one aggregated "retired" alert per sync batch when models
	// disappeared (GH #98, spec 0018 T4): info level, NULL endpoint_id,
	// message lists the newly-retired model IDs. The before/after diff ensures
	// only this batch's retirements are reported — models already retired by
	// an earlier sync are not re-announced.
	if retired > 0 {
		retiredAfter, err := s.db.ListRetiredModels(hub.ID)
		if err != nil {
			return stats, err
		}
		newlyRetired := diffModelIDs(retiredBefore, retiredAfter)
		if len(newlyRetired) > 0 {
			msg := fmt.Sprintf("Hub %q: %d model(s) retired (disappeared from listing): %s",
				hub.Name, len(newlyRetired), strings.Join(newlyRetired, ", "))
			if _, err := s.db.CreateAlertEvent(store.AlertEvent{
				Kind:    store.AlertKindRetired,
				Message: msg,
				SentOK:  true, // info-level retirement alerts are recorded, not sent
			}); err != nil {
				// Fail the sync: retirement alerts must be recorded (spec 0018 T4).
				// Swallowing the error would silently lose the retirement event.
				return stats, fmt.Errorf("create retirement alert: %w", err)
			}
		}
	}

	return stats, nil
}

// diffModelIDs returns the model IDs present in after but not in before.
func diffModelIDs(before, after []string) []string {
	seen := make(map[string]bool, len(before))
	for _, id := range before {
		seen[id] = true
	}
	var diff []string
	for _, id := range after {
		if !seen[id] {
			diff = append(diff, id)
		}
	}
	return diff
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

// SyncHubNow runs the same work as StartSync but synchronously on the
// caller: it takes the in-flight guard and persists the 'syncing' mark in
// the same order, then runs the sync inline and returns only after every
// tail write (sync result, task log) has landed. It exists for the server's
// WithSyncDiscovery test seam (ticket 100) — a structural synchronization
// point, because polling sync_status observes SetHubSyncResult, which
// precedes the task-log tail write. Production never calls this.
func (s *Syncer) SyncHubNow(ctx context.Context, hubID int64, source string) error {
	if !s.acquire(hubID) {
		return ErrSyncInProgress
	}
	defer s.release(hubID)
	if err := s.db.SetHubSyncing(hubID); err != nil {
		return err
	}
	hub, err := s.db.GetHub(hubID)
	if err != nil {
		return err
	}
	_, err = s.syncMarked(ctx, *hub, source)
	return err
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
	defer func() {
		if s.AfterSync != nil {
			s.AfterSync(hub.ID)
		}
	}()
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
	for _, protocol := range trialProtocolsFor(model.Capability) {
		if have[protocol] {
			continue
		}
		result := s.client.Probe(ctx, hub.BaseURL, hub.Token, protocol, model.ModelID, false, s.imageParamsFor(protocol, model.ModelID))
		if !result.OK {
			slog.Debug("discovery: protocol trial failed",
				"hub_id", hub.ID, "model", model.ModelID, "protocol", protocol,
				"http_status", result.HTTPStatus, "error", errSummary(result.ErrorSummary))
			continue
		}
		if ep, isNew, err := s.db.CreateEndpoint(model.ID, protocol, true); err != nil {
			return created, err
		} else if isNew {
			// A defaults-write failure leaves the endpoint on the global
			// interval: costly but recoverable via PATCH, and not worth
			// aborting the whole sync over (same policy as model trial).
			if err := s.db.ApplyCreationDefaults(ep.ID, protocol); err != nil {
				slog.Error("discovery: apply creation defaults",
					"hub_id", hub.ID, "model", model.ModelID, "protocol", protocol, "error", err)
			}
			created++
		}
	}
	return created, nil
}

// imageParamsFor resolves the rule-merged extra probe parameters for image
// protocol trials via the single resolution entry (store.ImageParamsFor,
// GH #33); chat trials take nil. A rules-table hiccup must never break a
// sync: on error the trial degrades to the minimal request body and the
// failure is logged.
func (s *Syncer) imageParamsFor(protocol, modelID string) map[string]string {
	if !hubclient.IsImageProtocol(protocol) {
		return nil
	}
	params, err := s.db.ImageParamsFor(modelID)
	if err != nil {
		slog.Warn("discovery: image param rules unavailable, trialing with minimal body",
			"model", modelID, "protocol", protocol, "error", err)
		return nil
	}
	return params
}

// errSummary dereferences a probe error summary for logging.
func errSummary(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
