package scheduler

import (
	"context"
	"log"
	"sync"
	"time"

	"git.github.net/taliove2009/ai-hub-checker/internal/evaluator"
	"git.github.net/taliove2009/ai-hub-checker/internal/store"
)

// defaultEvalPollInterval bounds how often the worker wakes to check whether
// the weekly eval window has opened.
const defaultEvalPollInterval = time.Minute

// weeklyEvalHourLimit defines "early morning": the weekly batch fires any
// time the local hour is below this on a Sunday.
const weeklyEvalHourLimit = 6

// EvalWorker runs a full evaluation batch every Sunday early morning (local
// time): one run per suite covering every active chat-capable model, with
// trigger="scheduled".
//
// It follows the RollupWorker pattern: a separate Clock-driven loop because
// the weekly cadence is unrelated to probe dispatch, and FakeClock-driven in
// tests. Firing is deduplicated by calendar date, so the hourly poll ticks
// inside the Sunday window produce exactly one batch per week.
type EvalWorker struct {
	db        *store.DB
	evaluator *evaluator.Evaluator
	clock     Clock

	pollInterval time.Duration

	mu          sync.Mutex
	lastRunDate string
}

// EvalWorkerOption customizes an EvalWorker.
type EvalWorkerOption func(*EvalWorker)

// WithEvalPollInterval overrides how often the worker checks the window.
func WithEvalPollInterval(d time.Duration) EvalWorkerOption {
	return func(w *EvalWorker) { w.pollInterval = d }
}

// NewEvalWorker creates a weekly eval worker over the given store, executing
// runs through evaluator and driven by clock.
func NewEvalWorker(db *store.DB, ev *evaluator.Evaluator, clock Clock, opts ...EvalWorkerOption) *EvalWorker {
	w := &EvalWorker{
		db:           db,
		evaluator:    ev,
		clock:        clock,
		pollInterval: defaultEvalPollInterval,
	}
	for _, opt := range opts {
		opt(w)
	}
	return w
}

// Run drives the weekly loop until ctx is canceled. The first tick runs
// immediately, so a service started inside the Sunday window catches up
// without waiting for the next poll.
func (w *EvalWorker) Run(ctx context.Context) {
	w.tick(ctx)
	for {
		timer := w.clock.NewTimer(w.pollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C():
		}
		w.tick(ctx)
	}
}

// tick fires the weekly batch when the clock sits inside the Sunday
// early-morning window and no batch ran on this date yet. Deduplication is
// two-level: an in-memory date for the common case, backed by a persistent
// check against eval_runs so a restart inside the window does not re-run a
// batch that already fired today.
func (w *EvalWorker) tick(ctx context.Context) {
	now := w.clock.Now()
	if now.Weekday() != time.Sunday || now.Hour() >= weeklyEvalHourLimit {
		return
	}
	date := now.Format("2006-01-02")

	w.mu.Lock()
	if w.lastRunDate == date {
		w.mu.Unlock()
		return
	}
	w.mu.Unlock()

	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	ran, err := w.db.HasScheduledEvalRunSince(startOfDay)
	if err != nil {
		log.Printf("eval worker: check scheduled runs: %v", err)
		return
	}
	if ran {
		w.mu.Lock()
		w.lastRunDate = date
		w.mu.Unlock()
		return
	}

	w.mu.Lock()
	if w.lastRunDate == date {
		w.mu.Unlock()
		return
	}
	w.lastRunDate = date
	w.mu.Unlock()

	w.runBatch(ctx)
}

// runBatch executes one scheduled run per suite against every active
// chat-capable model. Runs execute sequentially: the batch is weekly and
// small, and serial execution keeps hub load predictable. A suite with a
// failed run does not block the remaining suites.
func (w *EvalWorker) runBatch(ctx context.Context) {
	modelIDs, err := w.evalEligibleModels()
	if err != nil {
		log.Printf("eval worker: list models: %v", err)
		return
	}
	if len(modelIDs) == 0 {
		log.Printf("eval worker: no active chat models, skipping weekly batch")
		return
	}

	suites, err := w.db.ListSuites()
	if err != nil {
		log.Printf("eval worker: list suites: %v", err)
		return
	}

	judgeModel, err := w.db.GetSetting(store.SettingJudgeModel, store.DefaultJudgeModel)
	if err != nil {
		log.Printf("eval worker: read judge_model setting: %v", err)
		return
	}

	for _, suite := range suites {
		if ctx.Err() != nil {
			return
		}
		run, err := w.db.CreateEvalRun(suite.ID, "scheduled", judgeModel)
		if err != nil {
			log.Printf("eval worker: create run for suite %d: %v", suite.ID, err)
			continue
		}
		if err := w.evaluator.RunEval(ctx, run.ID, modelIDs); err != nil {
			log.Printf("eval worker: run suite %d: %v", suite.ID, err)
		}
	}
}

// evalEligibleModels returns the database IDs of all active, chat-capable
// models — the population the weekly batch evaluates.
func (w *EvalWorker) evalEligibleModels() ([]int64, error) {
	models, err := w.db.ListModels()
	if err != nil {
		return nil, err
	}
	var ids []int64
	for _, m := range models {
		if m.Status == "active" && m.Capability == "chat" {
			ids = append(ids, m.ID)
		}
	}
	return ids, nil
}
