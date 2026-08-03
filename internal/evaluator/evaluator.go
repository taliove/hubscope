// Package evaluator runs evaluation suites against models via the Hub and
// scores each answer either by rule (exact/regex/contains) or by an LLM
// judge following a rubric.
package evaluator

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/taliove/hubscope/internal/hubclient"
	"github.com/taliove/hubscope/internal/registry"
	"github.com/taliove/hubscope/internal/store"
)

// evalMaxTokens gives evaluated models room for a complete answer (unlike
// the 16-token probe budget).
const evalMaxTokens = 1024

// RequestTimeout bounds a single completion call during evaluation.
const RequestTimeout = 120 * time.Second

// circuitBreakerThreshold is the number of consecutive cases with failed
// answer calls after which a model's remaining cases are skipped (GH #153):
// a broken model burns ten calls per run (5 cases x 2 attempts) instead of
// two per case.
const circuitBreakerThreshold = 5

// campaignAbortCells is the number of completed cells after which an
// all-dead batch aborts (GH #153): when every completed cell produced zero
// answers, the Hub side is hopeless and unstarted cells are dropped.
const campaignAbortCells = 3

// Probe gate (spec 0020, GH #174): before the first case burns, every
// selected model is sampled probeGateRounds times with a tiny completion
// call; each round gets its own short timeout so a hung model cannot hold
// the gate, and probing fans out under a small cap.
const (
	probeGateRounds    = 3
	probeGateTimeout   = 30 * time.Second
	probeGateMaxTokens = 16
	probeGateParallel  = 8
	probeGatePrompt    = "ping"
)

// Evaluator executes eval runs and persists per-case results.
//
// AfterCampaign, when set, is invoked once after a campaign settles to
// "done" (never for failed campaigns). The score-drop alerter hooks in here;
// hook errors must be handled by the hook itself (the alerter logs instead
// of failing).
//
// Now, when set, replaces time.Now for the campaign budget deadline
// (GH #153); the server wires its own injectable clock here so tests can
// advance virtual time.
type Evaluator struct {
	db     *store.DB
	client *hubclient.Client

	AfterCampaign func(ctx context.Context, campaignID int64)
	Now           func() time.Time

	// cancels tracks the locally executing campaigns' cancel functions
	// (GH #152): every entry point (full sweep, single-suite run, retry,
	// weekly schedule) registers here, so POST /campaigns/{id}/cancel can
	// stop any of them.
	cancelMu sync.Mutex
	cancels  map[int64]context.CancelFunc

	// livePipelines maps a running campaign to its judge-stage pipeline
	// (GH #178): the campaign report reads the live queue depth of both
	// stages from it.
	livePipelines sync.Map // int64 → *pipeline
}

// New creates an Evaluator backed by the given store and hub client.
func New(db *store.DB, client *hubclient.Client) *Evaluator {
	return &Evaluator{db: db, client: client, cancels: map[int64]context.CancelFunc{}}
}

// registerCancel derives a cancelable context for one campaign's execution
// and records it until unregisterCancel (deferred by the caller).
func (e *Evaluator) registerCancel(ctx context.Context, campaignID int64) context.Context {
	ctx, cancel := context.WithCancel(ctx)
	e.cancelMu.Lock()
	e.cancels[campaignID] = cancel
	e.cancelMu.Unlock()
	return ctx
}

// unregisterCancel drops a campaign's cancel entry at execution end.
func (e *Evaluator) unregisterCancel(campaignID int64) {
	e.cancelMu.Lock()
	delete(e.cancels, campaignID)
	e.cancelMu.Unlock()
}

// CancelCampaign stops the locally executing campaign (GH #152): in-flight
// cells run to completion, unstarted cells are dropped and their runs fail,
// so the campaign settles failed through the normal machinery. It reports
// false when the campaign is not executing on this process (already
// canceled, settled, or a stale running row from a crashed process).
func (e *Evaluator) CancelCampaign(campaignID int64) bool {
	e.cancelMu.Lock()
	defer e.cancelMu.Unlock()
	cancel, ok := e.cancels[campaignID]
	if ok {
		delete(e.cancels, campaignID)
		cancel()
	}
	return ok
}

// now returns the current time from the injected clock, defaulting to the
// wall clock.
func (e *Evaluator) now() time.Time {
	if e.Now != nil {
		return e.Now()
	}
	return time.Now()
}

// RunEval executes every enabled case of the run's suite against each
// selected model and marks the run done. A failing model never blocks the
// others; per-case failures are recorded as results with nil scores.
//
// Execution goes through the same (run × model) cell pool as full campaigns
// (GH #26): one cell per selected model, bounded by the configured
// eval_concurrency. The judge model is read from settings at run start
// (default store.DefaultJudgeModel); the run record is updated when the
// configured value differs from the snapshot taken at creation, so
// eval_runs.judge_model always reflects the judge actually used. The default
// sample count is read the same way; a case's own sample_count overrides it.
func (e *Evaluator) RunEval(ctx context.Context, runID int64, modelDBIDs []int64) error {
	prep, err := e.prepareRun(runID, len(modelDBIDs))
	if err != nil {
		return err
	}
	// Registered for operator cancel under the run's campaign (GH #152).
	ctx = e.registerCancel(ctx, prep.run.CampaignID)
	defer e.unregisterCancel(prep.run.CampaignID)
	e.executePrepared(ctx, []*preparedRun{prep}, modelDBIDs)
	return ctx.Err()
}

// preparedRun is a run staged for execution: the judge-resolved run record,
// its task-center mirror and the enabled cases of its suite, loaded once so
// every (run × model) cell of the run shares them.
type preparedRun struct {
	run   *store.EvalRun
	task  *runTask
	cases []store.Case
}

// prepareRun loads and stages one run for cell execution. A case-loading
// failure marks the run failed (with its task) and returns the error, the
// same outcome the serial executor produced.
func (e *Evaluator) prepareRun(runID int64, modelCount int) (*preparedRun, error) {
	run, err := e.db.GetEvalRun(runID)
	if err != nil {
		return nil, fmt.Errorf("load eval run %d: %w", runID, err)
	}
	run = e.resolveJudgeModel(run)

	// Register the run with the task center; tracking failures never abort
	// the run itself (beginRunTask returns a no-op logger then).
	task := e.beginRunTask(run, modelCount)

	cases, err := e.db.ListEnabledCases(run.SuiteID)
	if err != nil {
		_ = e.db.FinishEvalRun(runID, "failed", time.Now().UTC())
		task.fail(fmt.Sprintf("load cases for suite %d: %v", run.SuiteID, err))
		return nil, fmt.Errorf("load cases for suite %d: %w", run.SuiteID, err)
	}
	return &preparedRun{run: run, task: task, cases: cases}, nil
}

// evalCell is the unit of eval execution (GH #26): one run against one
// model. Cases stay serial inside a cell, keeping per-model cadence and hub
// pressure predictable; cells fan out across a bounded worker pool.
type evalCell struct {
	prep      *preparedRun
	modelDBID int64
}

// executePrepared fans every (run × model) cell of the prepared runs out to
// the bounded worker pool and finishes each run once its cells complete:
// done when every cell executed, failed when the context was canceled with
// cells unstarted. It returns after all cells executed or the context was
// canceled (in-flight cells always run to completion).
//
// Cells are fed model-major (GH #169, the #161/#166 map decision): every
// suite of model 1, then every suite of model 2, and so on. A model's hub
// pressure and failure cadence (circuit breaker, retry) stay contiguous
// instead of being spread across the whole campaign, and a broken model is
// found after its first suite instead of burning one cell per suite.
func (e *Evaluator) executePrepared(ctx context.Context, prepared []*preparedRun, modelDBIDs []int64) {
	defaultSamples := e.resolveDefaultSampleCount()

	// GH #155 (decision B): judge calls ride the evaluated model's hub.
	// Name every evaluated hub that lacks the judge up front — multi-hub
	// deployments must not learn about an unreachable judge from a wiped
	// leaderboard.
	e.warnJudgeUnreachable(prepared, modelDBIDs)

	// The GH #153 guards (campaign budget, all-dead abort) share the
	// existing cancellation machinery: stopping means canceling, with an
	// explicit reason carried to the runs whose cells never started.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Probe gate (spec 0020, GH #174): measure every selected model's
	// reachability before the first case burns. Unreachable models leave
	// the feed — no cells, no dead rows — and an all-unreachable batch
	// fails every run with the gate reason instead of settling an empty
	// success. The gate runs inside the campaign budget: probing is part
	// of the batch's wall clock. The probe population includes jury
	// candidates on the same hubs (GH #175): the jury ranking needs their
	// measured speed, and a candidate is cheap to sample.
	samples := e.probeGate(ctx, prepared, modelDBIDs, e.juryProbePopulation(modelDBIDs))
	// A nil gate means the context was canceled mid-probe: the outcome is
	// meaningless, so feed everything and let the canceled pool drop every
	// cell — the standard tail then fails the runs with the cancellation
	// reason instead of a bogus gate verdict.
	feed := modelDBIDs
	if samples != nil {
		feed = make([]int64, 0, len(modelDBIDs))
		for _, id := range modelDBIDs {
			if samples[id].reachable {
				feed = append(feed, id)
			}
		}
		if len(feed) == 0 && len(modelDBIDs) > 0 {
			for _, prep := range prepared {
				e.failPreparedRun(prep, "probe gate: every selected model unreachable")
			}
			return
		}
		// Jury selection (spec 0020, GH #175): one jury per subject from
		// its own hub's reachable candidates, ranked by the configured
		// policy; the snapshot lands on every run before the first cell.
		e.selectJuries(prepared, feed, samples)
	}

	var deadline time.Time
	if budget := e.resolveCampaignBudgetMinutes(); budget > 0 {
		deadline = e.now().Add(time.Duration(budget) * time.Minute)
	}

	var mu sync.Mutex
	stopReason := ""
	stop := func(reason string) {
		mu.Lock()
		if stopReason == "" {
			stopReason = reason
		}
		mu.Unlock()
		cancel()
	}
	budgetExceeded := func() bool {
		return !deadline.IsZero() && !e.now().Before(deadline)
	}

	p := newPipeline(e, prepared)
	remaining := map[int64]int{}
	for _, prep := range prepared {
		remaining[prep.run.ID] = len(feed)
		// A run with no models has no cells to wait for: it evaluates
		// nothing and finishes done immediately (the serial executor's
		// zero-model outcome).
		if len(feed) == 0 {
			p.markCellsDone(prep.run.ID)
		}
	}
	// GH #169: model-major cell order — the pool takes a model's whole suite
	// list before the next model's first cell.
	var cells []evalCell
	for _, modelDBID := range feed {
		for _, prep := range prepared {
			cells = append(cells, evalCell{prep: prep, modelDBID: modelDBID})
		}
	}

	var completed, dead int
	p.runJudgePool(ctx, budgetExceeded)
	p.setCellsTotal(len(cells))
	if len(prepared) > 0 {
		campaignID := prepared[0].run.CampaignID
		e.livePipelines.Store(campaignID, p)
		defer e.livePipelines.Delete(campaignID)
	}
	e.runCellPool(ctx, cells, func() bool {
		if budgetExceeded() {
			stop("campaign budget exceeded")
			return true
		}
		return false
	}, func(cctx context.Context, cell evalCell) {
		p.noteCellStart()
		answered := e.evalModel(cctx, p, cell.prep, cell.modelDBID, cell.prep.cases, cell.prep.task, defaultSamples)
		p.noteCellDone()
		mu.Lock()
		remaining[cell.prep.run.ID]--
		done := remaining[cell.prep.run.ID] == 0
		completed++
		if !answered {
			dead++
		}
		abort := completed >= campaignAbortCells && dead == completed
		expired := budgetExceeded()
		external := ctx.Err() != nil
		mu.Unlock()
		// An external cancellation keeps its own reason: the guards only
		// name a stop they caused, never blame the Hub for an operator's
		// cancel.
		if abort && !external {
			stop("campaign aborted: every completed cell failed")
		}
		if expired && !external {
			stop("campaign budget exceeded")
		}
		if done {
			// All of the run's exam cells executed: the run finishes once
			// its judge queue is also empty (GH #176).
			p.markCellsDone(cell.prep.run.ID)
		}
	})
	// The exam stage is done (or canceled): stop the judge feed and wait
	// for in-flight judge calls. Votes never offered stay owed in the
	// database — the recovery sweep sees them.
	p.closeJudgeQueue()

	// Cells dropped because the context was canceled never reported back;
	// their runs fail, matching the serial executor's cancellation outcome.
	// Runs whose judge queue could not drain fail the same way: their case
	// grid is incomplete.
	mu.Lock()
	defer mu.Unlock()
	for _, prep := range prepared {
		if p.isFinished(prep.run.ID) {
			continue
		}
		reason := stopReason
		if reason == "" {
			reason = "execution incomplete"
			if err := ctx.Err(); err != nil {
				reason = err.Error()
			}
		}
		e.failPreparedRun(prep, reason)
	}
}

// runCellPool executes job for every cell with at most eval_concurrency
// workers at once (GH #26). A canceled context stops workers from taking new
// cells and stops the feeder from offering them; cells already in flight run
// to completion before runCellPool returns. guard, when non-nil, is an
// extra stop predicate evaluated at the same points (the campaign budget,
// GH #153); it may cancel the context to carry its reason. Result
// persistence stays safe under fan-out: the store's single SQLite
// connection serializes writes (W2 — the pool never widens it).
func (e *Evaluator) runCellPool(ctx context.Context, cells []evalCell, guard func() bool, job func(ctx context.Context, cell evalCell)) {
	if len(cells) == 0 {
		return
	}
	workers := e.resolveEvalConcurrency()
	if workers > len(cells) {
		workers = len(cells)
	}
	stopped := func() bool {
		return ctx.Err() != nil || (guard != nil && guard())
	}

	jobs := make(chan evalCell)
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				// Pre-check: once the context is canceled, no new cell is
				// taken — without it a select with both branches ready
				// (canceled ctx + pending cell) would randomly steal one.
				if stopped() {
					return
				}
				select {
				case <-ctx.Done():
					return
				case cell, ok := <-jobs:
					if !ok {
						return
					}
					job(ctx, cell)
				}
			}
		}()
	}

feed:
	for _, cell := range cells {
		if stopped() {
			break feed
		}
		select {
		case <-ctx.Done():
			break feed
		case jobs <- cell:
		}
	}
	close(jobs)
	wg.Wait()
}

// finishPreparedRun stamps a terminal status onto the run and mirrors it to
// the task center.
func (e *Evaluator) finishPreparedRun(prep *preparedRun, status string) {
	if err := e.db.FinishEvalRun(prep.run.ID, status, time.Now().UTC()); err != nil {
		prep.task.fail(err.Error())
		return
	}
	prep.task.succeed()
}

// failPreparedRun marks the run failed with the given reason, in the store
// and in the task center.
func (e *Evaluator) failPreparedRun(prep *preparedRun, reason string) {
	_ = e.db.FinishEvalRun(prep.run.ID, "failed", time.Now().UTC())
	prep.task.fail(reason)
}

// SettleCampaign settles a campaign from its member runs and, when the
// campaign reached "done", invokes the AfterCampaign hook. It is the single
// settle entry point shared by the sweep loop and the single-run trigger, so
// every done campaign produces exactly one hook invocation: terminal campaign
// states are sticky (the first settle wins) and each campaign is settled by
// exactly one goroutine.
//
// The hook runs detached from cancellation so a graceful shutdown cannot
// abort an alert send mid-flight, and a panicking hook is logged rather than
// allowed to take down the caller.
func (e *Evaluator) SettleCampaign(ctx context.Context, campaignID int64) {
	if err := e.db.SettleCampaign(campaignID, time.Now().UTC()); err != nil {
		slog.Error("evaluator: settle campaign", "campaign_id", campaignID, "error", err)
		return
	}
	if e.AfterCampaign == nil {
		return
	}
	campaign, err := e.db.GetCampaign(campaignID)
	if err != nil {
		slog.Error("evaluator: reload settled campaign", "campaign_id", campaignID, "error", err)
		return
	}
	if campaign.Status != store.CampaignStatusDone {
		return
	}
	func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("evaluator: AfterCampaign hook panicked", "campaign_id", campaignID, "panic", r)
			}
		}()
		e.AfterCampaign(context.WithoutCancel(ctx), campaignID)
	}()
}

// resolveJudgeModel reads the configured judge model from settings and, when
// it differs from the run's snapshot, persists it onto the run. It returns a
// copy of the run carrying the effective judge model; the input is not
// mutated. Read/write failures fall back to the snapshot.
func (e *Evaluator) resolveJudgeModel(run *store.EvalRun) *store.EvalRun {
	judgeModel, err := e.db.GetSetting(store.SettingJudgeModel, store.DefaultJudgeModel)
	if err != nil {
		slog.Error("evaluator: read judge_model setting, keeping run snapshot", "error", err)
		return run
	}
	if judgeModel == "" || judgeModel == run.JudgeModel {
		return run
	}
	if err := e.db.SetEvalRunJudgeModel(run.ID, judgeModel); err != nil {
		slog.Error("evaluator: record judge_model on run", "run_id", run.ID, "error", err)
		return run
	}
	updated := *run
	updated.JudgeModel = judgeModel
	return &updated
}

// resolveDefaultSampleCount reads the global default sample count from
// settings, clamped to [1, store.MaxSampleCount]. Read failures fall back to
// the built-in default.
func (e *Evaluator) resolveDefaultSampleCount() int {
	n, err := e.db.GetSettingInt(store.SettingDefaultSampleCount, store.DefaultSampleCount)
	if err != nil {
		slog.Error("evaluator: read default_sample_count setting, using default", "error", err)
		return store.DefaultSampleCount
	}
	if n < 1 {
		return 1
	}
	if n > store.MaxSampleCount {
		return store.MaxSampleCount
	}
	return n
}

// resolveEvalConcurrency reads the eval worker-pool size from settings,
// clamped to [1, store.MaxEvalConcurrency] (GH #26). Read failures fall back
// to the built-in default. The clamp mirrors the settings API validation, so
// a hand-edited database value can never widen the pool past the cap.
func (e *Evaluator) resolveEvalConcurrency() int {
	n, err := e.db.GetSettingInt(store.SettingEvalConcurrency, store.DefaultEvalConcurrency)
	if err != nil {
		slog.Error("evaluator: read eval_concurrency setting, using default", "error", err)
		return store.DefaultEvalConcurrency
	}
	if n < 1 {
		return 1
	}
	if n > store.MaxEvalConcurrency {
		return store.MaxEvalConcurrency
	}
	return n
}

// warnJudgeUnreachable logs one warn line per (run, hub) whose judge
// model is missing from that hub (GH #155, decision B). Judge calls ride
// the evaluated model's hub, so a judge present on Hub A does nothing for
// Hub B's models; the task log must say so before the cases burn.
func (e *Evaluator) warnJudgeUnreachable(prepared []*preparedRun, modelDBIDs []int64) {
	hasJudge := false
	for _, prep := range prepared {
		for _, c := range prep.cases {
			if c.VerdictType == "judge" {
				hasJudge = true
				break
			}
		}
	}
	if !hasJudge {
		return
	}

	// Distinct hubs behind the evaluated models.
	hubs := map[int64]string{}
	for _, id := range modelDBIDs {
		model, err := e.db.GetModel(id)
		if err != nil {
			continue
		}
		if _, seen := hubs[model.HubID]; seen {
			continue
		}
		hub, err := e.db.GetHub(model.HubID)
		if err != nil {
			continue
		}
		hubs[model.HubID] = hub.Name
	}

	reachable := map[string]bool{}
	for hubID, hubName := range hubs {
		for _, prep := range prepared {
			judge := prep.run.JudgeModel
			key := fmt.Sprintf("%d|%s", hubID, judge)
			ok, seen := reachable[key]
			if !seen {
				var err error
				ok, err = e.db.ModelExistsOnHub(hubID, judge)
				if err != nil {
					ok = true // a check failure must not cry wolf
				}
				reachable[key] = ok
			}
			if !ok {
				prep.task.log(store.TaskLogWarn, fmt.Sprintf(
					"judge model %q unreachable on hub %q: judge cases for its models will stay unjudged",
					judge, hubName))
			}
		}
	}
}

// probeSample is what the gate learned about one model: reachability (at
// least one successful round), the successful-round count, and the average
// measured output speed. Jury selection consumes tps (spec 0020); the cell
// feed consumes reachable.
type probeSample struct {
	reachable bool
	successes int
	tps       float64
}

// probeGate measures each selected model's reachability before any case
// burns (spec 0020, GH #174): probeGateRounds small completion calls per
// model, models probed in parallel under a small cap. The outcome decides
// the cell feed — an unreachable subject is skipped without cells or
// result rows (the GH #154 retired-model form) — and is named in every
// run's task log. probeIDs may exceed subjectIDs (jury candidates on the
// same hubs, GH #175): candidates are sampled for the jury ranking but
// never gate anything. Outcomes live only in the returned map and the
// task logs: they never touch the probes table or the status machine
// (W5 — an eval probe is not monitoring). Models that cannot be probed
// (gone, retired, no enabled chat endpoint) stay reachable so evalModel's
// existing setup-failure paths report them unchanged.
func (e *Evaluator) probeGate(ctx context.Context, prepared []*preparedRun, subjectIDs, probeIDs []int64) map[int64]probeSample {
	subjects := make(map[int64]bool, len(subjectIDs))
	for _, id := range subjectIDs {
		subjects[id] = true
	}
	type probeTarget struct {
		id       int64
		hub      *store.Hub
		protocol string
		modelID  string
	}
	samples := make(map[int64]probeSample, len(probeIDs))
	var targets []probeTarget
	for _, id := range probeIDs {
		samples[id] = probeSample{reachable: true}
		model, err := e.db.GetModel(id)
		if err != nil || model.Status == "retired" {
			continue
		}
		hub, err := e.db.GetHub(model.HubID)
		if err != nil {
			continue
		}
		endpoints, err := e.db.ListEndpointsByModelID(id)
		if err != nil {
			continue
		}
		protocol, ok := selectProtocol(endpoints)
		if !ok {
			continue
		}
		targets = append(targets, probeTarget{id: id, hub: hub, protocol: protocol, modelID: model.ModelID})
	}

	var mu sync.Mutex
	// The gate fans out under the same hub-pressure budget as the cell
	// pool (capped by probeGateParallel): an admin who dialed
	// eval_concurrency down to 1 asked for serial pressure, probes
	// included.
	parallel := e.resolveEvalConcurrency()
	if parallel > probeGateParallel {
		parallel = probeGateParallel
	}
	sem := make(chan struct{}, parallel)
	var wg sync.WaitGroup
	for _, tgt := range targets {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			sample := e.probeModel(ctx, tgt.hub, tgt.protocol, tgt.modelID)
			if ctx.Err() != nil {
				// A canceled context makes the probe outcome meaningless:
				// never stamp a model unreachable on a canceled round —
				// the caller aborts the whole gate instead.
				return
			}
			mu.Lock()
			samples[tgt.id] = sample
			mu.Unlock()
			if sample.reachable || !subjects[tgt.id] {
				return
			}
			for _, prep := range prepared {
				prep.task.log(store.TaskLogWarn, fmt.Sprintf(
					"model %s unreachable at probe gate (0/%d), skipped: no cases burned",
					tgt.modelID, probeGateRounds))
			}
		}()
	}
	wg.Wait()
	if ctx.Err() != nil {
		return nil
	}
	return samples
}

// probeModel takes probeGateRounds reachability samples of one model — the
// full round count, never an early exit: the sample set doubles as the
// model's stability and speed signal (spec 0020), and a fixed cost keeps
// batch call-count accounting deterministic. Reachable means at least one
// success; each round gets its own timeout so a hung model cannot stall
// the gate.
func (e *Evaluator) probeModel(ctx context.Context, hub *store.Hub, protocol, modelID string) probeSample {
	var sample probeSample
	var tpsSum float64
	var tpsRounds int
	for range probeGateRounds {
		if ctx.Err() != nil {
			return probeSample{}
		}
		rctx, cancel := context.WithTimeout(ctx, probeGateTimeout)
		res := e.client.Complete(rctx, hub.BaseURL, hub.Token, protocol, modelID, probeGatePrompt, probeGateMaxTokens)
		cancel()
		if !res.OK {
			continue
		}
		sample.successes++
		// A sub-millisecond localhost round is a success too — it just
		// carries no speed signal.
		if res.LatencyMs > 0 {
			outTokens := probeGateMaxTokens
			if res.OutputTokens != nil && *res.OutputTokens > 0 {
				outTokens = *res.OutputTokens
			}
			tpsSum += float64(outTokens) / (float64(res.LatencyMs) / 1000)
			tpsRounds++
		}
	}
	sample.reachable = sample.successes > 0
	if tpsRounds > 0 {
		sample.tps = tpsSum / float64(tpsRounds)
	}
	return sample
}

// juryProbePopulation returns the probe population for a batch (spec 0020,
// GH #175): the selected subjects plus every other non-retired chat model
// on their hubs — the jury candidate pool. Candidates are probed for the
// ranking inputs (reachability, measured speed) but never gate anything.
func (e *Evaluator) juryProbePopulation(modelDBIDs []int64) []int64 {
	seen := map[int64]bool{}
	var out []int64
	add := func(id int64) {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	for _, id := range modelDBIDs {
		add(id)
	}
	hubs := map[int64]bool{}
	for _, id := range modelDBIDs {
		model, err := e.db.GetModel(id)
		if err != nil || hubs[model.HubID] {
			continue
		}
		hubs[model.HubID] = true
		models, err := e.db.ListModelsByHub(model.HubID)
		if err != nil {
			continue
		}
		for _, m := range models {
			if m.Status == "retired" {
				continue
			}
			endpoints, err := e.db.ListEndpointsByModelID(m.ID)
			if err != nil {
				continue
			}
			if _, ok := selectProtocol(endpoints); ok {
				add(m.ID)
			}
		}
	}
	return out
}

// resolveJuryPolicy reads the configured jury policy, falling back to the
// default on read failure or an unknown stored value.
func (e *Evaluator) resolveJuryPolicy() string {
	policy, err := e.db.GetSetting(store.SettingJuryPolicy, store.DefaultJuryPolicy)
	if err != nil || !ValidJuryPolicy(policy) {
		if err != nil {
			slog.Error("evaluator: read jury_policy setting, using default", "error", err)
		}
		return store.DefaultJuryPolicy
	}
	return policy
}

// resolveRegistryOverrides reads the administrator's registry overrides;
// a corrupt stored value falls back to the built-in table (the settings
// read path's fail-open caliber).
func (e *Evaluator) resolveRegistryOverrides() []registry.Override {
	raw, err := e.db.GetSetting(store.SettingModelRegistryOverrides, "")
	if err != nil {
		slog.Error("evaluator: read model_registry_overrides, using built-ins", "error", err)
		return nil
	}
	overrides, err := registry.ParseOverrides(raw)
	if err != nil {
		slog.Error("evaluator: corrupt model_registry_overrides, using built-ins", "error", err)
		return nil
	}
	return overrides
}

// selectJuries picks one jury per subject (spec 0020, GH #175): candidates
// are the subject's own-hub reachable chat models, ranked by the configured
// policy over registry IQ, gate-measured speed and registry price. The
// subject is excluded when at least three alternatives exist; a short or
// self-including jury is logged, never fatal. Every run snapshots the full
// selection before its first cell (ADR 0016), and the task log names the
// judges so a surprising pick is visible before the cases burn.
func (e *Evaluator) selectJuries(prepared []*preparedRun, feed []int64, samples map[int64]probeSample) {
	policy := e.resolveJuryPolicy()
	overrides := e.resolveRegistryOverrides()

	// Hub candidates are shared across subjects on the same hub.
	byHub := map[int64][]juryCandidate{}
	subjects := map[int64]*store.Model{}
	for _, id := range feed {
		model, err := e.db.GetModel(id)
		if err != nil {
			continue
		}
		subjects[id] = model
		if _, done := byHub[model.HubID]; done {
			continue
		}
		var cands []juryCandidate
		models, err := e.db.ListModelsByHub(model.HubID)
		if err != nil {
			continue
		}
		for _, m := range models {
			sample, probed := samples[m.ID]
			if !probed || !sample.reachable {
				continue
			}
			info := registry.Lookup(m.ModelID, overrides)
			cands = append(cands, juryCandidate{
				ModelDBID: m.ID,
				ModelID:   m.ModelID,
				IQ:        info.IQ,
				PriceIn:   info.PriceIn,
				PriceOut:  info.PriceOut,
				TPS:       sample.tps,
			})
		}
		byHub[model.HubID] = cands
	}

	juries := map[int64]jurySelection{}
	for id, model := range subjects {
		sel := selectJury(byHub[model.HubID], policy, model.ModelID)
		juries[id] = sel
		switch {
		case len(sel.Judges) == 0:
			for _, prep := range prepared {
				prep.task.log(store.TaskLogWarn, fmt.Sprintf(
					"model %s: no reachable jury candidates on its hub — judge cases will stay unjudged", model.ModelID))
			}
		default:
			note := ""
			if sel.SelfIncluded {
				note = " WARNING: subject serves on its own jury (self-preference bias)"
			}
			if len(sel.Judges) < 3 {
				note += fmt.Sprintf(" short jury: %d judge(s)", len(sel.Judges))
			}
			for _, prep := range prepared {
				prep.task.log(store.TaskLogInfo, fmt.Sprintf(
					"model %s jury (%s): %s%s", model.ModelID, policy, strings.Join(sel.Judges, ", "), note))
			}
		}
	}

	snapshot := jurySnapshotJSON(policy, juries)
	for _, prep := range prepared {
		if err := e.db.SetEvalRunJuryModels(prep.run.ID, snapshot); err != nil {
			slog.Error("evaluator: snapshot jury on run", "run_id", prep.run.ID, "error", err)
		}
		// Keep the in-memory record in step: judgesFor reads it when the
		// cells fan out, before any later store reload.
		prep.run.JuryModels = snapshot
	}
}

// LiveQueueDepth reports the running campaign's queue state across both
// pipeline stages (GH #178); ok is false when no batch is executing for
// the campaign on this process.
func (e *Evaluator) LiveQueueDepth(campaignID int64) (examPending, examInflight, judgePending, judgeInflight int, ok bool) {
	v, ok := e.livePipelines.Load(campaignID)
	if !ok {
		return 0, 0, 0, 0, false
	}
	examPending, examInflight, judgePending, judgeInflight = v.(*pipeline).queueDepth()
	return examPending, examInflight, judgePending, judgeInflight, true
}

// resolveJudgeConcurrency reads the judge-stage pool size from settings,
// clamped to [1, store.MaxEvalConcurrency] (GH #176). Read failures fall
// back to the built-in default.
func (e *Evaluator) resolveJudgeConcurrency() int {
	n, err := e.db.GetSettingInt(store.SettingJudgeConcurrency, store.DefaultJudgeConcurrency)
	if err != nil {
		slog.Error("evaluator: read judge_concurrency setting, using default", "error", err)
		return store.DefaultJudgeConcurrency
	}
	if n < 1 {
		return 1
	}
	if n > store.MaxEvalConcurrency {
		return store.MaxEvalConcurrency
	}
	return n
}

// resolveCampaignBudgetMinutes reads the campaign wall-clock budget from
// settings, clamped to [0, store.MaxEvalCampaignBudgetMin] (GH #153); 0
// disables the budget. Read failures fall back to the built-in default.
func (e *Evaluator) resolveCampaignBudgetMinutes() int {
	n, err := e.db.GetSettingInt(store.SettingEvalCampaignBudgetMin, store.DefaultEvalCampaignBudgetMin)
	if err != nil {
		slog.Error("evaluator: read eval_campaign_budget_minutes setting, using default", "error", err)
		return store.DefaultEvalCampaignBudgetMin
	}
	if n < 0 {
		return 0
	}
	if n > store.MaxEvalCampaignBudgetMin {
		return store.MaxEvalCampaignBudgetMin
	}
	return n
}

// evalModel runs all cases against one model. Any setup failure (model gone,
// hub gone, no enabled endpoint) is recorded as failed results for every
// case so the model x case grid stays complete, and logged to the task.
// The per-model circuit breaker (GH #153): after circuitBreakerThreshold
// consecutive cases whose answer calls all failed, the remaining cases are
// recorded unscored with the circuit reason instead of burning two more Hub
// calls each. The pre-flight skip reads the status board's down caliber
// before the first case: a model whose enabled chat endpoints are all down
// is skipped without any call or result row. The return reports whether
// any case got an answer at all — the campaign-level abort reads it to
// detect an all-dead batch.
func (e *Evaluator) evalModel(ctx context.Context, p *pipeline, prep *preparedRun, modelDBID int64, cases []store.Case, task *runTask, defaultSamples int) bool {
	run := prep.run
	model, err := e.db.GetModel(modelDBID)
	if err != nil {
		e.failAllCases(run, modelDBID, "", cases, "model not found")
		task.log(store.TaskLogWarn, fmt.Sprintf("model db_id=%d skipped: model not found", modelDBID))
		return false
	}
	// Retired between trigger and cell start (GH #154): skip without
	// calls and without stamping dead rows every view must filter out —
	// the retirement was an operator decision, not a failure.
	if model.Status == "retired" {
		task.log(store.TaskLogWarn, fmt.Sprintf("model %s skipped: retired", model.ModelID))
		return false
	}

	hub, err := e.db.GetHub(model.HubID)
	if err != nil {
		e.failAllCases(run, modelDBID, model.ModelID, cases, "hub not found")
		task.log(store.TaskLogWarn, fmt.Sprintf("model %s skipped: hub not found", model.ModelID))
		return false
	}

	endpoints, err := e.db.ListEndpointsByModelID(modelDBID)
	if err != nil {
		e.failAllCases(run, modelDBID, model.ModelID, cases, "failed to load endpoints")
		task.log(store.TaskLogWarn, fmt.Sprintf("model %s skipped: failed to load endpoints", model.ModelID))
		return false
	}

	protocol, ok := selectProtocol(endpoints)
	if !ok {
		e.failAllCases(run, modelDBID, model.ModelID, cases, "no enabled endpoint for this model")
		task.log(store.TaskLogWarn, fmt.Sprintf("model %s skipped: no enabled endpoint for this model", model.ModelID))
		return false
	}

	// Pre-flight offline skip: when the status board already shows every
	// enabled chat endpoint of the model as down, each answer call would
	// 503 into the circuit breaker — a broken model burns ten Hub calls
	// per run before it opens. Skip like the retired precedent (GH #154):
	// no dead rows, one warn line, and a false return so the cell still
	// counts toward the all-dead campaign abort. A failed check never
	// skips the model (fail-open): the circuit breaker stays the backstop.
	down, err := e.db.ListModelsAllChatEndpointsDown([]int64{modelDBID})
	if err != nil {
		task.log(store.TaskLogWarn, fmt.Sprintf("model %s: endpoint-down pre-flight check failed: %v", model.ModelID, err))
	} else if down[modelDBID] {
		task.log(store.TaskLogWarn, fmt.Sprintf("model %s skipped: endpoints down (pre-flight)", model.ModelID))
		return false
	}

	answered := false
	consecutiveFailed := 0
	for i, c := range cases {
		if e.evalCase(ctx, p, prep, hub, protocol, model, c, task, defaultSamples) {
			answered = true
			consecutiveFailed = 0
			continue
		}
		consecutiveFailed++
		if consecutiveFailed >= circuitBreakerThreshold && i+1 < len(cases) {
			reason := fmt.Sprintf("circuit open: %d consecutive answer failures", circuitBreakerThreshold)
			e.failAllCases(run, modelDBID, model.ModelID, cases[i+1:], reason)
			task.log(store.TaskLogWarn, fmt.Sprintf("model %s: %s, skipping remaining %d cases", model.ModelID, reason, len(cases)-i-1))
			break
		}
	}
	return answered
}

// evalCase answers one case sampleCount times through the exam stage and
// routes each sample to its verdict path (spec 0020, GH #176): rule
// verdicts settle inline, judge samples persist to eval_answers and fan
// out to the jury — the case's eval_results row is written by the pipeline
// when its last sample settles. The return reports whether any sample got
// an answer (the circuit breaker counts consecutive unanswered cases).
func (e *Evaluator) evalCase(ctx context.Context, p *pipeline, prep *preparedRun, hub *store.Hub, protocol string, model *store.Model, c store.Case, task *runTask, defaultSamples int) bool {
	run := prep.run
	samples := defaultSamples
	if c.SampleCount != nil && *c.SampleCount >= 1 {
		samples = *c.SampleCount
	}

	// Judge cases move to the jury-median caliber (ADR 0016) only when the
	// run carries a jury snapshot; legacy runs keep the single-judge V2
	// caliber they were created with.
	judges, hasJury := e.judgesFor(run, model.ID)
	profile := VerdictProfileCurrent
	if c.VerdictType == "judge" && hasJury {
		profile = store.VerdictProfileV3
	}
	p.openCase(prep, model.ID, c.ID, model.ModelID, profile, samples)

	answered := false
	for i := 1; i <= samples; i++ {
		out := e.examSample(ctx, run, hub, protocol, model, c, i)
		if out.answer == nil {
			p.settleSample(run.ID, model.ID, c.ID, nil, fmt.Sprintf("sample %d/%d: %s", i, samples, out.detail))
			continue
		}
		answered = true
		p.recordExamCost(run.ID, model.ModelID, out.inputTokens, out.outputTokens)
		p.recordAnswer(run.ID, model.ID, c.ID, out.latencyMs, out.inputTokens, out.outputTokens, *out.answer)
		if c.VerdictType == "rule" {
			score, detail := ruleVerdict(c, *out.answer, profile)
			if out.detail != "answered" {
				detail = out.detail + "; " + detail
			}
			p.settleSample(run.ID, model.ID, c.ID, score, fmt.Sprintf("sample %d/%d: %s", i, samples, detail))
			continue
		}
		if len(judges) == 0 {
			p.settleSample(run.ID, model.ID, c.ID, nil, fmt.Sprintf("sample %d/%d: no jury available", i, samples))
			continue
		}
		p.enqueueVotes(ctx, judgeJob{
			prep:            prep,
			model:           model,
			hub:             hub,
			protocol:        protocol,
			c:               c,
			answerID:        out.answerID,
			sampleNo:        i,
			expectedSamples: samples,
			answerText:      *out.answer,
		}, judges)
	}
	return answered
}

// judgesFor resolves the judge list for one model from the run's jury
// snapshot (spec 0020). The second return reports whether a snapshot
// exists at all: legacy runs (no snapshot) fall back to the single
// judge_model and keep the V2 caliber.
func (e *Evaluator) judgesFor(run *store.EvalRun, modelDBID int64) ([]string, bool) {
	_, juries := parseJurySnapshot(run.JuryModels)
	if juries == nil {
		return []string{run.JudgeModel}, false
	}
	return juries[modelDBID], true
}

// examOutcome is one answer-call attempt's exam-stage result: the answer
// text (nil when every attempt failed), the eval_answers row ID, and the
// accounting payload.
type examOutcome struct {
	answerID     int64
	answer       *string
	detail       string
	latencyMs    int
	inputTokens  *int
	outputTokens *int
}

// examSample executes one answer call and persists it to eval_answers —
// before any judge call exists, so a crash never loses a paid completion
// (ADR 0016). A failed call (timeout, connection error, hub 5xx) is
// retried exactly once, immediately (GH #27); a canceled context skips
// the retry.
func (e *Evaluator) examSample(ctx context.Context, run *store.EvalRun, hub *store.Hub, protocol string, model *store.Model, c store.Case, sampleNo int) examOutcome {
	res := e.client.Complete(ctx, hub.BaseURL, hub.Token, protocol, model.ModelID, c.Prompt, evalMaxTokens)
	retried := false
	if !res.OK && ctx.Err() == nil {
		// Second and final attempt. A canceled context skips the retry — the
		// retry would fail the same way and the detail must stay honest.
		retry := e.client.Complete(ctx, hub.BaseURL, hub.Token, protocol, model.ModelID, c.Prompt, evalMaxTokens)
		retried = true
		if retry.OK {
			res = retry
		} else {
			res.LatencyMs += retry.LatencyMs
			res.ErrorSummary = retry.ErrorSummary
		}
	}
	out := examOutcome{
		latencyMs:    res.LatencyMs,
		inputTokens:  res.InputTokens,
		outputTokens: res.OutputTokens,
	}

	status := store.EvalAnswerAnswered
	if !res.OK {
		status = store.EvalAnswerFailed
	}
	row := store.EvalAnswer{
		EvalRunID: run.ID, ModelDBID: model.ID, ModelID: model.ModelID,
		CaseID: c.ID, SampleNo: sampleNo, Status: status,
		LatencyMs: res.LatencyMs, InputTokens: res.InputTokens, OutputTokens: res.OutputTokens,
	}
	if res.OK {
		row.AnswerText = &res.Text
	}
	answerID, err := e.db.CreateEvalAnswer(row)
	if err != nil {
		slog.Error("evaluator: persist answer", "run_id", run.ID, "case_id", c.ID, "error", err)
	}

	if !res.OK {
		out.detail = "answer call failed"
		if res.ErrorSummary != nil {
			out.detail = "answer call failed: " + *res.ErrorSummary
		}
		if retried {
			out.detail = "answer call failed after 2 attempts"
			if res.ErrorSummary != nil {
				out.detail = "answer call failed after 2 attempts: " + *res.ErrorSummary
			}
		}
		return out
	}

	out.answerID = answerID
	out.answer = &res.Text
	out.detail = "answered"
	if retried {
		out.detail = "answered on attempt 2 after an initial failure"
	}
	return out
}

// addIntPtr sums two nullable ints; null only when both sides are null.
func addIntPtr(a, b *int) *int {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	sum := *a + *b
	return &sum
}

// failAllCases records a failed result (no answer, no score) for every case.
// Failed results carry the current profile too, so a run's caliber can always
// be derived from any of its rows. Whole-set failure stamps go through one
// batch transaction (GH #150).
func (e *Evaluator) failAllCases(run *store.EvalRun, modelDBID int64, modelID string, cases []store.Case, reason string) {
	rows := make([]store.EvalResult, 0, len(cases))
	for _, c := range cases {
		rows = append(rows, store.EvalResult{
			EvalRunID:      run.ID,
			ModelDBID:      modelDBID,
			ModelID:        modelID,
			CaseID:         c.ID,
			VerdictDetail:  &reason,
			VerdictProfile: VerdictProfileCurrent,
		})
	}
	if err := e.db.CreateEvalResultsBatch(rows); err != nil {
		slog.Error("evaluator: persist failure stamp", "run_id", run.ID, "model_db_id", modelDBID, "error", err)
	}
}

// storeResult persists one result; persistence errors are logged but never
// abort the whole run.
func (e *Evaluator) storeResult(r store.EvalResult) {
	if _, err := e.db.CreateEvalResult(r); err != nil {
		slog.Error("evaluator: persist result for case", "case_id", r.CaseID, "error", err)
	}
}

// selectProtocol picks the chat protocol used to call a model: anthropic
// when its endpoint is enabled, otherwise openai. Image-protocol endpoints
// are never selected (spec 0014, R1 guard): an evaluation prompt sent to an
// image endpoint would burn a paid generation per case and pollute scoring
// with non-answers, so a model without an enabled chat endpoint reports "no
// enabled endpoint" instead.
func selectProtocol(endpoints []store.Endpoint) (string, bool) {
	for _, ep := range endpoints {
		if ep.Enabled && ep.Protocol == "anthropic" {
			return "anthropic", true
		}
	}
	for _, ep := range endpoints {
		if ep.Enabled && ep.Protocol == "openai" {
			return "openai", true
		}
	}
	return "", false
}
