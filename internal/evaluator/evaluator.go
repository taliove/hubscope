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

	remaining := map[int64]int{}
	var cells []evalCell
	for _, prep := range prepared {
		remaining[prep.run.ID] = len(modelDBIDs)
		for _, modelDBID := range modelDBIDs {
			cells = append(cells, evalCell{prep: prep, modelDBID: modelDBID})
		}
		// A run with no models has no cells to wait for: it evaluates
		// nothing and finishes done immediately (the serial executor's
		// zero-model outcome).
		if len(modelDBIDs) == 0 {
			e.finishPreparedRun(prep, "done")
		}
	}

	var completed, dead int
	e.runCellPool(ctx, cells, func() bool {
		if budgetExceeded() {
			stop("campaign budget exceeded")
			return true
		}
		return false
	}, func(cctx context.Context, cell evalCell) {
		answered := e.evalModel(cctx, cell.prep.run, cell.modelDBID, cell.prep.cases, cell.prep.task, defaultSamples)
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
			// All of the run's cells executed: the run is done regardless
			// of a later cancellation (the serial executor finished a run
			// done once its last model completed, too).
			e.finishPreparedRun(cell.prep, "done")
		}
	})

	// Cells dropped because the context was canceled never reported back;
	// their runs fail, matching the serial executor's cancellation outcome.
	mu.Lock()
	defer mu.Unlock()
	for _, prep := range prepared {
		if remaining[prep.run.ID] > 0 {
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
// calls each. The return reports whether any case got an answer at all —
// the campaign-level abort reads it to detect an all-dead batch.
func (e *Evaluator) evalModel(ctx context.Context, run *store.EvalRun, modelDBID int64, cases []store.Case, task *runTask, defaultSamples int) bool {
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

	answered := false
	consecutiveFailed := 0
	for i, c := range cases {
		if e.evalCase(ctx, run, hub, protocol, model, c, task, defaultSamples) {
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

// evalCase answers one case sampleCount times and stores a single result
// whose score is the average of the judged samples. Samples that cannot be
// judged (answer call failed, judge failed) contribute no score; when no
// sample is judged at all the case stays unscored — the same convention as a
// single unjudged answer. The outcome is logged to the task: scored
// completions at info, answer/judge failures at warn. The return reports
// whether any sample got an answer (the circuit breaker counts consecutive
// unanswered cases).
func (e *Evaluator) evalCase(ctx context.Context, run *store.EvalRun, hub *store.Hub, protocol string, model *store.Model, c store.Case, task *runTask, defaultSamples int) bool {
	samples := defaultSamples
	if c.SampleCount != nil && *c.SampleCount >= 1 {
		samples = *c.SampleCount
	}

	result := store.EvalResult{
		EvalRunID:      run.ID,
		ModelDBID:      model.ID,
		ModelID:        model.ModelID,
		CaseID:         c.ID,
		VerdictProfile: VerdictProfileCurrent,
	}

	var scoreSum float64
	var scored int
	answered := false
	var details []string
	for i := 1; i <= samples; i++ {
		sample := e.evalSample(ctx, run, hub, protocol, model, c)
		result.LatencyMs += sample.latencyMs
		result.InputTokens = addIntPtr(result.InputTokens, sample.inputTokens)
		result.OutputTokens = addIntPtr(result.OutputTokens, sample.outputTokens)
		if sample.answer != nil {
			answered = true
			if result.AnswerText == nil {
				result.AnswerText = sample.answer
			}
		}
		if sample.score != nil {
			scoreSum += *sample.score
			scored++
		}
		details = append(details, fmt.Sprintf("sample %d/%d: %s", i, samples, sample.detail))
	}

	if scored > 0 {
		avg := scoreSum / float64(scored)
		result.Score = &avg
	}
	detail := strings.Join(details, "; ")
	result.VerdictDetail = &detail
	e.storeResult(result)

	switch {
	case result.Score != nil:
		task.log(store.TaskLogInfo, fmt.Sprintf("case %d done: model=%s score=%.2f", c.ID, model.ModelID, *result.Score))
	case strings.Contains(detail, "judge"):
		task.log(store.TaskLogWarn, fmt.Sprintf("case %d judge failed: model=%s detail=%q", c.ID, model.ModelID, detail))
	default:
		task.log(store.TaskLogWarn, fmt.Sprintf("case %d failed: model=%s detail=%q", c.ID, model.ModelID, detail))
	}
	return answered
}

// sampleOutcome is one answer-and-verdict attempt for a case.
type sampleOutcome struct {
	answer       *string
	score        *float64
	detail       string
	latencyMs    int
	inputTokens  *int
	outputTokens *int
}

// evalSample executes one answer call plus its verdict. A failed answer call
// (timeout, connection error, hub 5xx) is retried exactly once, immediately —
// the 120s request timeout is already the wait (GH #27). The judge call is
// never retried: a failed judge stays a null score (W7).
func (e *Evaluator) evalSample(ctx context.Context, run *store.EvalRun, hub *store.Hub, protocol string, model *store.Model, c store.Case) sampleOutcome {
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
	out := sampleOutcome{
		latencyMs:    res.LatencyMs,
		inputTokens:  res.InputTokens,
		outputTokens: res.OutputTokens,
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

	out.answer = &res.Text
	out.score, out.detail = e.verdict(ctx, hub, protocol, run.JudgeModel, c, res.Text)
	if retried {
		// Never claim a first-try success: the detail names the recovery.
		out.detail += "; answer succeeded on attempt 2 after an initial failure"
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

// verdict scores an answer according to the case's verdict type. Rule
// verdicts run under the current verdict profile (ADR 0008).
func (e *Evaluator) verdict(ctx context.Context, hub *store.Hub, protocol, judgeModel string, c store.Case, answer string) (*float64, string) {
	if c.VerdictType == "rule" {
		return ruleVerdict(c, answer, VerdictProfileCurrent)
	}
	return e.judgeVerdict(ctx, hub, protocol, judgeModel, c, answer)
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
