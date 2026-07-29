package server

import (
	"context"
	"encoding/json"
	"hash/fnv"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/taliove/hubscope/internal/store"
)

// overview_snapshot.go implements spec 0015 decisions 3+4: the /api/overview
// response is served from a process-wide in-memory snapshot (W5 single
// evaluator/alerter precedent) instead of recomputing 5 SQL queries per
// endpoint per request on the single SQLite connection.
//
// Freshness model — a cached snapshot is served O(1) while ALL of its
// markers still hold, and rebuilt (singleflight per scope) when any moves:
//
//   - structGen: bumped by InvalidateOverview, the single invalidation entry
//     point every structural write path calls (endpoint add/delete/enable,
//     model create/delete/trial, hub create/update/delete, discovery sync
//     completion, eval campaign settle, probe rounds via HandleProbeRound).
//   - probe watermark: MAX(id) of the probes table, one indexed query per
//     request. It catches probe writes from ANY path — including ones that
//     bypass the handlers (seeded history in W1 tests, future writers) — so
//     the dirty-flag enumeration can never silently lag the status board.
//   - hour bucket: the snapshot is rebuilt when s.now() (W4 injected clock)
//     crosses an hour boundary, so the 24h windows and hour-aligned dots
//     decay on time even when nothing is written (idle endpoints).
//
// W5 semantics are untouched: the status machine still derives from probe
// history on every rebuild; the snapshot is a read-path precomputation only.

// overviewScopeAll selects every model (super_admin and the anonymous public
// status board). Hub IDs start at 1, so 0 and -1 never collide with a real
// hub scope.
const overviewScopeAll = 0

// overviewScopeEmpty selects no models: a hub-scoped user without a hub_id
// (data inconsistency) gets an empty overview rather than the full set.
const overviewScopeEmpty = -1

// overviewSnapshot is one immutable, fully serialized overview response plus
// the freshness markers it was built under. Readers share it without locks
// beyond the cache's short map access.
type overviewSnapshot struct {
	body      []byte    // serialized {"data": overviewDTO} envelope
	etag      string    // quoted content hash of body
	watermark int64     // probes MAX(id) after the build
	structGen uint64    // structural-write generation after the build
	hour      time.Time // s.now() truncated to the hour at build time
}

// overviewRebuild is the singleflight call per scope: concurrent pollers
// that find the snapshot stale wait on one leader's rebuild.
type overviewRebuild struct {
	done chan struct{}
	snap *overviewSnapshot
	err  error
}

// overviewCache holds the per-scope snapshots and the structural-write
// generation. The mutex guards map access and structGen only; DTO builds
// happen outside it.
type overviewCache struct {
	mu        sync.Mutex
	snaps     map[int64]*overviewSnapshot
	inflight  map[int64]*overviewRebuild
	structGen uint64
}

func newOverviewCache() *overviewCache {
	return &overviewCache{
		snaps:    map[int64]*overviewSnapshot{},
		inflight: map[int64]*overviewRebuild{},
	}
}

// InvalidateOverview is the single invalidation entry point (spec 0015
// decision 3): every write path that can change the overview content calls
// it. It only bumps the structural generation — a cheap dirty flag; the next
// reader rebuilds lazily, so writes never pay for a board nobody is reading.
func (s *Server) InvalidateOverview() {
	c := s.overview
	c.mu.Lock()
	c.structGen++
	c.mu.Unlock()
}

// overviewScopeKey maps the request's session to its snapshot scope:
// super_admin and anonymous (public status board) see all models; hub-scoped
// roles see only their hub's models. Same selection as listModelsForRequest.
func overviewScopeKey(r *http.Request) int64 {
	u := sessionUser(r)
	if u == nil || u.Role == store.RoleSuperAdmin {
		return overviewScopeAll
	}
	if u.HubID == nil {
		return overviewScopeEmpty
	}
	return *u.HubID
}

// overviewSnapshotFor returns a fresh snapshot for the request's scope,
// serving the cached one when every marker still holds and collapsing
// concurrent rebuilds into one (singleflight).
func (s *Server) overviewSnapshotFor(r *http.Request) (*overviewSnapshot, error) {
	scope := overviewScopeKey(r)
	watermark, err := s.db.ProbeWatermark()
	if err != nil {
		return nil, err
	}
	hour := s.now().UTC().Truncate(time.Hour)

	c := s.overview
	c.mu.Lock()
	if snap := c.snaps[scope]; snap != nil &&
		snap.watermark == watermark &&
		snap.structGen == c.structGen &&
		snap.hour.Equal(hour) {
		c.mu.Unlock()
		return snap, nil
	}
	if call, ok := c.inflight[scope]; ok {
		c.mu.Unlock()
		<-call.done
		return call.snap, call.err
	}
	call := &overviewRebuild{done: make(chan struct{})}
	c.inflight[scope] = call
	c.mu.Unlock()

	snap, err := c.rebuild(s, scope)

	c.mu.Lock()
	delete(c.inflight, scope)
	if err == nil {
		c.snaps[scope] = snap
	}
	call.snap, call.err = snap, err
	close(call.done)
	c.mu.Unlock()
	return snap, err
}

// rebuild computes the DTO and serializes it into a snapshot. To never store
// a snapshot that claims to be fresher than its content, the markers are
// read before AND after the build: if a write landed mid-build they moved
// and the build is redone (bounded — under a sustained write storm the last
// attempt is stored with post-build markers, which only risks one extra
// rebuild on the next request, the safe direction).
func (c *overviewCache) rebuild(s *Server, scope int64) (*overviewSnapshot, error) {
	for attempt := 0; ; attempt++ {
		wm0, err := s.db.ProbeWatermark()
		if err != nil {
			return nil, err
		}
		c.mu.Lock()
		gen0 := c.structGen
		c.mu.Unlock()

		now := s.now().UTC()
		models, err := s.listModelsForScope(scope)
		if err != nil {
			return nil, err
		}
		dto, err := s.buildOverviewDTO(models, now)
		if err != nil {
			return nil, err
		}
		body, err := json.Marshal(map[string]interface{}{"data": dto})
		if err != nil {
			return nil, err
		}

		wm1, err := s.db.ProbeWatermark()
		if err != nil {
			return nil, err
		}
		c.mu.Lock()
		gen1 := c.structGen
		c.mu.Unlock()

		if (wm0 == wm1 && gen0 == gen1) || attempt >= 2 {
			hash := fnv.New64a()
			_, _ = hash.Write(body)
			return &overviewSnapshot{
				body:      body,
				etag:      `"` + strconv.FormatUint(hash.Sum64(), 16) + `"`,
				watermark: wm1,
				structGen: gen1,
				hour:      now.Truncate(time.Hour),
			}, nil
		}
	}
}

// ifNoneMatchMatches reports whether the If-None-Match header contains the
// snapshot's ETag (or the wildcard), per RFC 9110 section 13.1.2.
func ifNoneMatchMatches(header, etag string) bool {
	for _, token := range strings.Split(header, ",") {
		token = strings.TrimSpace(token)
		if token == "*" || token == etag {
			return true
		}
	}
	return false
}

// HandleProbeRound is the prober AfterRound hook for BOTH the server's own
// prober (manual rounds) and main's scheduler prober: alert evaluation first
// (W5: one evaluator per process, manual and scheduled rounds share the
// semantics), then overview invalidation (a probe round always moves the
// status board's data).
func (s *Server) HandleProbeRound(ctx context.Context, endpointID int64, results []store.Probe) {
	s.alerter.HandleRound(ctx, endpointID, results)
	s.InvalidateOverview()
}

// handleCampaignSettled is the evaluator AfterCampaign hook: score-drop
// alerts first, then overview invalidation (a settled campaign moves the
// eval_score column of the overview).
func (s *Server) handleCampaignSettled(ctx context.Context, campaignID int64) {
	s.alerter.HandleCampaign(ctx, campaignID)
	s.InvalidateOverview()
}
