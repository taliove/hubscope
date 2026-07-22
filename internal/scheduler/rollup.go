package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/taliove2009/hubscope/internal/store"
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
	// defaultAuditRetention is how long audit log entries are kept.
	defaultAuditRetention = 90 * 24 * time.Hour
	// defaultTaskRetention is how long finished tasks and their logs are
	// kept before the daily cleanup prunes them.
	defaultTaskRetention = 90 * 24 * time.Hour
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
	auditRetention  time.Duration
	taskRetention   time.Duration
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

// WithAuditRetention overrides how long audit log entries are kept.
func WithAuditRetention(d time.Duration) RollupOption {
	return func(w *RollupWorker) { w.auditRetention = d }
}

// WithTaskRetention overrides how long finished tasks and their logs are
// kept.
func WithTaskRetention(d time.Duration) RollupOption {
	return func(w *RollupWorker) { w.taskRetention = d }
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
		auditRetention:  defaultAuditRetention,
		taskRetention:   defaultTaskRetention,
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
// aggregated into rollups. Both jobs register a task in the task center with
// the rows they processed; tracking failures never break the job itself.
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
		cutoff := now.Add(-w.rollupLag)
		tracker := w.db.BeginTask(store.TaskTypeRollup, store.TaskSourceScheduled, "", 0,
			fmt.Sprintf("rollup started: cutoff=%s", cutoff.UTC().Format(time.RFC3339)))
		aggregated, err := w.db.RollupProbesBefore(cutoff)
		if err != nil {
			slog.Error("rollup worker: rollup probes", "error", err)
			tracker.Fail("rollup failed: " + err.Error())
		} else {
			tracker.Succeed(fmt.Sprintf("rollup finished: probes_aggregated=%d", aggregated))
		}
	}
	if dueCleanup {
		probesCutoff := now.Add(-w.retention)
		auditCutoff := now.Add(-w.auditRetention)
		tracker := w.db.BeginTask(store.TaskTypeRetentionCleanup, store.TaskSourceScheduled, "", 0,
			fmt.Sprintf("retention cleanup started: probes_before=%s audit_before=%s",
				probesCutoff.UTC().Format(time.RFC3339), auditCutoff.UTC().Format(time.RFC3339)))

		failed := false
		probesDeleted, err := w.db.DeleteProbesBefore(probesCutoff)
		if err != nil {
			slog.Error("rollup worker: cleanup probes", "error", err)
			tracker.Log(store.TaskLogError, "delete raw probes failed: "+err.Error())
			failed = true
		} else {
			tracker.Log(store.TaskLogInfo, fmt.Sprintf("deleted raw probes: probes_deleted=%d", probesDeleted))
		}
		auditPruned, err := w.db.PruneAuditLogsBefore(auditCutoff)
		if err != nil {
			slog.Error("rollup worker: prune audit logs", "error", err)
			tracker.Log(store.TaskLogError, "prune audit logs failed: "+err.Error())
			failed = true
		} else {
			tracker.Log(store.TaskLogInfo, fmt.Sprintf("pruned audit logs: audit_logs_pruned=%d", auditPruned))
		}
		tasksPruned, err := w.db.PruneTasksBefore(now.Add(-w.taskRetention))
		if err != nil {
			slog.Error("rollup worker: prune tasks", "error", err)
			tracker.Log(store.TaskLogError, "prune tasks failed: "+err.Error())
			failed = true
		} else {
			tracker.Log(store.TaskLogInfo, fmt.Sprintf("pruned tasks: tasks_pruned=%d", tasksPruned))
		}

		if failed {
			tracker.Fail("retention cleanup finished with errors")
		} else {
			tracker.Succeed(fmt.Sprintf("retention cleanup finished: probes_deleted=%d audit_logs_pruned=%d tasks_pruned=%d", probesDeleted, auditPruned, tasksPruned))
		}
	}
}
