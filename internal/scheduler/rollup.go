package scheduler

import (
	"context"
	"log"
	"sync"
	"time"

	"git.github.net/taliove2009/ai-hub-checker/internal/store"
)

// Cadence defaults for the rollup worker.
const (
	// defaultRollupInterval is how often old probes are aggregated.
	defaultRollupInterval = time.Hour
	// defaultCleanupInterval is how often retention cleanup runs.
	defaultCleanupInterval = 24 * time.Hour
	// defaultRetention is how long raw probe rows are kept.
	defaultRetention = 90 * 24 * time.Hour
	// defaultRollupLag is how old a probe must be before it is rolled up, so
	// the raw tail stays available for the recent-failures list.
	defaultRollupLag = time.Hour
	// defaultRollupPollInterval bounds how often the worker wakes to check
	// whether rollup or cleanup is due.
	defaultRollupPollInterval = time.Minute
)

// RollupWorker periodically aggregates old probes into hourly rollups and
// deletes raw probes past the retention window.
//
// It is a separate loop from the probe Scheduler on purpose: rollup and
// cleanup run on an hourly/daily cadence unrelated to per-endpoint probe
// dispatch, and folding them into the scheduler's tight dispatch loop would
// couple slow batch SQL to probe latency. It reuses the same Clock
// abstraction, so tests drive it with a FakeClock exactly like the scheduler.
type RollupWorker struct {
	db              *store.DB
	clock           Clock
	rollupInterval  time.Duration
	cleanupInterval time.Duration
	retention       time.Duration
	rollupLag       time.Duration
	pollInterval    time.Duration

	mu          sync.Mutex
	lastRollup  time.Time
	lastCleanup time.Time
}

// RollupOption customizes a RollupWorker.
type RollupOption func(*RollupWorker)

// WithRollupInterval overrides how often rollup runs.
func WithRollupInterval(d time.Duration) RollupOption {
	return func(w *RollupWorker) { w.rollupInterval = d }
}

// WithCleanupInterval overrides how often retention cleanup runs.
func WithCleanupInterval(d time.Duration) RollupOption {
	return func(w *RollupWorker) { w.cleanupInterval = d }
}

// WithRetention overrides how long raw probes are kept.
func WithRetention(d time.Duration) RollupOption {
	return func(w *RollupWorker) { w.retention = d }
}

// WithRollupLag overrides how old a probe must be before it is rolled up.
func WithRollupLag(d time.Duration) RollupOption {
	return func(w *RollupWorker) { w.rollupLag = d }
}

// WithRollupPollInterval overrides how often the worker wakes to check dues.
func WithRollupPollInterval(d time.Duration) RollupOption {
	return func(w *RollupWorker) { w.pollInterval = d }
}

// NewRollupWorker creates a worker over the given store, driven by clock.
func NewRollupWorker(db *store.DB, clock Clock, opts ...RollupOption) *RollupWorker {
	w := &RollupWorker{
		db:              db,
		clock:           clock,
		rollupInterval:  defaultRollupInterval,
		cleanupInterval: defaultCleanupInterval,
		retention:       defaultRetention,
		rollupLag:       defaultRollupLag,
		pollInterval:    defaultRollupPollInterval,
	}
	for _, opt := range opts {
		opt(w)
	}
	return w
}

// Run drives the rollup/cleanup loop until ctx is canceled. The first tick
// runs immediately at startup so a fresh instance backfills rollups without
// waiting a full interval.
func (w *RollupWorker) Run(ctx context.Context) {
	w.tick()
	for {
		timer := w.clock.NewTimer(w.pollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C():
		}
		w.tick()
	}
}

// tick runs rollup when due, then cleanup when due. Rollup always precedes
// cleanup so retention never deletes raw probes that have not yet been
// aggregated into rollups.
func (w *RollupWorker) tick() {
	now := w.clock.Now()

	w.mu.Lock()
	dueRollup := now.Sub(w.lastRollup) >= w.rollupInterval
	dueCleanup := now.Sub(w.lastCleanup) >= w.cleanupInterval
	if dueRollup {
		w.lastRollup = now
	}
	if dueCleanup {
		w.lastCleanup = now
	}
	w.mu.Unlock()

	if dueRollup {
		if err := w.db.RollupProbesBefore(now.Add(-w.rollupLag)); err != nil {
			log.Printf("rollup worker: rollup probes: %v", err)
		}
	}
	if dueCleanup {
		if _, err := w.db.DeleteProbesBefore(now.Add(-w.retention)); err != nil {
			log.Printf("rollup worker: cleanup probes: %v", err)
		}
	}
}
