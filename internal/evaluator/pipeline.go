package evaluator

// The decoupled judge stage of the eval pipeline (spec 0020, GH #176/GH
// #177). The exam stage (evalModel/evalCase in evaluator.go) answers each
// sample, persists it to eval_answers, and enqueues one judgeJob per jury
// slot; the judge pool below drains that queue, writes one eval_judge_scores
// row per call, and aggregates medians into the eval_results row when a case
// completes. Answers land before any judge call, so a crash never loses paid
// completions: the recovery sweep rebuilds the queue from under-judged
// answers and re-issues only the missing slots.

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"

	"github.com/taliove/hubscope/internal/registry"
	"github.com/taliove/hubscope/internal/store"
)

// judgeJob is one judge call owed to one jury slot.
type judgeJob struct {
	prep            *preparedRun
	model           *store.Model
	hub             *store.Hub
	protocol        string
	c               store.Case
	answerID        int64
	sampleNo        int
	expectedSamples int
	slot            int
	judgeModel      string
	answerText      string
}

// caseAggregate accumulates one (run, model, case) cell's outcomes until
// the case's eval_results row can be written. The exam stage opens it with
// the case metadata, records each answered sample's payload, and settles
// samples as they complete — rule samples and answer failures settle at
// exam time, judge samples settle when their votes are all in.
type caseAggregate struct {
	prep            *preparedRun
	modelID         string
	profile         string
	expectedSamples int
	settledSamples  int
	scoreSum        float64
	scored          int
	latencyMs       int
	inputTokens     *int
	outputTokens    *int
	answerText      *string
	details         []string
}

// sampleVotes tracks one judge sample's jury votes until the median can be
// taken. scores is slot-indexed; a failed judge leaves a nil vote (W7).
type sampleVotes struct {
	expected int
	landed   int
	scores   []*float64
	details  []string
}

// pipeline is the per-batch judge stage: a bounded worker pool fed through
// a channel by the exam stage, plus the aggregate state that turns votes
// into case results. A run finishes when its exam cells are exhausted AND
// its judge queue is empty. finishRun fires at that point — the batch
// executor's run finisher by default, the retry driver's stamp on the
// retry path.
type pipeline struct {
	e         *Evaluator
	jobs      chan judgeJob
	wg        sync.WaitGroup
	finishRun func(prep *preparedRun)

	mu           sync.Mutex
	preps        map[int64]*preparedRun
	aggs         map[[3]int64]*caseAggregate // (run, model, case)
	votes        map[[4]int64]*sampleVotes   // (run, model, case, sample)
	judgePending map[int64]int               // outstanding judge jobs per run
	cellsDone    map[int64]bool
	finished     map[int64]bool
	costs        map[int64]*costState // per-run estimated USD cost (GH #178)
	overrides    []registry.Override

	// Live queue accounting for the running campaign's report (GH #178):
	// exam counters track the cell pool, judge counters the vote flow.
	// The by-model judge maps feed the ops monitor table (GH #179).
	examTotal, examStarted, examDone    int
	judgeOffered, judgeTaken, judgeDone int
	judgeOfferedByModel                 map[int64]int
	judgeDoneByModel                    map[int64]int
}

// costState accumulates one run's estimated cost: exam (answer calls) and
// judge (jury calls) portions. unknown marks any component whose price or
// token usage is unregistered — the run's cost then reads as NULL ("price
// not registered"), never as a partial sum presented as complete.
type costState struct {
	exam    float64
	judge   float64
	unknown bool
}

func newPipeline(e *Evaluator, prepared []*preparedRun) *pipeline {
	p := &pipeline{
		e:                   e,
		jobs:                make(chan judgeJob),
		preps:               map[int64]*preparedRun{},
		aggs:                map[[3]int64]*caseAggregate{},
		votes:               map[[4]int64]*sampleVotes{},
		judgePending:        map[int64]int{},
		cellsDone:           map[int64]bool{},
		finished:            map[int64]bool{},
		costs:               map[int64]*costState{},
		judgeOfferedByModel: map[int64]int{},
		judgeDoneByModel:    map[int64]int{},
	}
	for _, prep := range prepared {
		p.preps[prep.run.ID] = prep
	}
	p.finishRun = func(prep *preparedRun) { e.finishPreparedRun(prep, "done") }
	p.overrides = e.resolveRegistryOverrides()
	return p
}

// openCase registers (idempotently) a case's aggregate with its metadata.
func (p *pipeline) openCase(prep *preparedRun, modelDBID, caseID int64, modelID, profile string, expectedSamples int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	key := [3]int64{prep.run.ID, modelDBID, caseID}
	if _, ok := p.aggs[key]; !ok {
		p.aggs[key] = &caseAggregate{prep: prep, modelID: modelID, profile: profile, expectedSamples: expectedSamples}
	}
}

// recordAnswer folds an answered sample's payload (latency, tokens, first
// answer text) into its case aggregate without settling the sample — the
// judge stage settles it once the votes land.
func (p *pipeline) recordAnswer(runID, modelDBID, caseID int64, latencyMs int, inputTokens, outputTokens *int, answerText string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	agg := p.aggs[[3]int64{runID, modelDBID, caseID}]
	agg.latencyMs += latencyMs
	agg.inputTokens = addIntPtr(agg.inputTokens, inputTokens)
	agg.outputTokens = addIntPtr(agg.outputTokens, outputTokens)
	if agg.answerText == nil {
		text := answerText
		agg.answerText = &text
	}
}

// settleSample marks one sample complete and, when the case's samples are
// all settled, writes its eval_results row. median is nil for unscored
// samples (W7); detail is the sample's human-readable account.
func (p *pipeline) settleSample(runID, modelDBID, caseID int64, median *float64, detail string) {
	p.mu.Lock()
	key := [3]int64{runID, modelDBID, caseID}
	agg := p.aggs[key]
	agg.settledSamples++
	if median != nil {
		agg.scoreSum += *median
		agg.scored++
	}
	agg.details = append(agg.details, detail)
	complete := agg.settledSamples == agg.expectedSamples
	var snapshot caseAggregate
	if complete {
		snapshot = *agg
		delete(p.aggs, key)
	}
	p.mu.Unlock()

	if complete {
		p.writeCaseResult(runID, modelDBID, caseID, &snapshot)
	}
}

// writeCaseResult persists one completed case's eval_results row: the score
// is the mean of the sample medians, null when no sample scored (W7).
func (p *pipeline) writeCaseResult(runID, modelDBID, caseID int64, agg *caseAggregate) {
	result := store.EvalResult{
		EvalRunID:      runID,
		ModelDBID:      modelDBID,
		ModelID:        agg.modelID,
		CaseID:         caseID,
		VerdictProfile: agg.profile,
		LatencyMs:      agg.latencyMs,
		InputTokens:    agg.inputTokens,
		OutputTokens:   agg.outputTokens,
		AnswerText:     agg.answerText,
	}
	if agg.scored > 0 {
		avg := agg.scoreSum / float64(agg.scored)
		result.Score = &avg
	}
	detail := strings.Join(agg.details, "; ")
	result.VerdictDetail = &detail
	p.e.storeResult(result)

	if agg.prep == nil {
		return // recovery path: the run's task settled with the crash
	}
	switch {
	case result.Score != nil:
		agg.prep.task.log(store.TaskLogInfo, fmt.Sprintf("case %d done: model=%s score=%.2f", caseID, agg.modelID, *result.Score))
	case strings.Contains(detail, "=FAIL") || strings.Contains(detail, "unjudged"):
		agg.prep.task.log(store.TaskLogWarn, fmt.Sprintf("case %d judge failed: model=%s detail=%q", caseID, agg.modelID, detail))
	default:
		agg.prep.task.log(store.TaskLogWarn, fmt.Sprintf("case %d failed: model=%s detail=%q", caseID, agg.modelID, detail))
	}
}

// enqueueVotes registers a judge sample's votes and offers one job per
// judge to the pool. A canceled context stops the offer mid-fan-out: jobs
// already taken run, jobs not offered stay owed in the database (the
// answer row keeps them visible to the recovery sweep).
func (p *pipeline) enqueueVotes(ctx context.Context, job judgeJob, judges []string) {
	key := [4]int64{job.prep.run.ID, job.model.ID, job.c.ID, int64(job.sampleNo)}
	p.mu.Lock()
	p.votes[key] = &sampleVotes{expected: len(judges), scores: make([]*float64, len(judges))}
	p.judgePending[job.prep.run.ID] += len(judges)
	p.judgeOfferedByModel[job.model.ID] += len(judges)
	p.mu.Unlock()
	for slot, judgeModel := range judges {
		j := job
		j.slot = slot
		j.judgeModel = judgeModel
		select {
		case <-ctx.Done():
			return
		case p.jobs <- j:
			p.mu.Lock()
			p.judgeOffered++
			p.mu.Unlock()
		}
	}
}

// runJudgePool drains the judge queue with at most judge_concurrency
// workers (GH #176). A canceled context stops workers from taking new
// jobs; jobs in flight run to completion.
func (p *pipeline) runJudgePool(ctx context.Context, guard func() bool) {
	workers := p.e.resolveJudgeConcurrency()
	stopped := func() bool { return ctx.Err() != nil || (guard != nil && guard()) }
	for range workers {
		p.wg.Add(1)
		go func() {
			defer p.wg.Done()
			for {
				if stopped() {
					return
				}
				select {
				case <-ctx.Done():
					return
				case job, ok := <-p.jobs:
					if !ok {
						return
					}
					p.mu.Lock()
					p.judgeTaken++
					p.mu.Unlock()
					p.executeJudge(ctx, job)
					p.mu.Lock()
					p.judgeDone++
					p.judgeDoneByModel[job.model.ID]++
					p.mu.Unlock()
				}
			}
		}()
	}
}

// closeJudgeQueue stops the feed, waits for in-flight judge calls, and
// persists every run's cost estimate.
func (p *pipeline) closeJudgeQueue() {
	close(p.jobs)
	p.wg.Wait()
	p.flushCosts()
}

// executeJudge performs one judge call, records its row, and folds the
// vote. The call is never retried (W7): a failed judge leaves a NULL score
// row for its slot.
func (p *pipeline) executeJudge(ctx context.Context, job judgeJob) {
	score, detail, inTok, outTok := p.e.judgeVerdict(ctx, job.hub, job.protocol, job.judgeModel, job.c, job.answerText)
	p.recordJudgeCost(job.prep.run.ID, job.judgeModel, inTok, outTok)
	if _, err := p.e.db.CreateEvalJudgeScore(store.EvalJudgeScore{
		AnswerID:     job.answerID,
		Slot:         job.slot,
		JudgeModel:   job.judgeModel,
		Score:        score,
		InputTokens:  inTok,
		OutputTokens: outTok,
	}); err != nil {
		job.prep.task.log(store.TaskLogWarn, fmt.Sprintf("persist judge score for answer %d slot %d: %v", job.answerID, job.slot, err))
	}
	if score == nil {
		job.prep.task.log(store.TaskLogWarn, fmt.Sprintf(
			"judge %s failed case %d sample %d (slot stays null, W7): %s", job.judgeModel, job.c.ID, job.sampleNo, detail))
	}
	p.landVote(job, score, detail)
}

// landVote folds one judge's verdict into its sample; when the sample's
// votes are all in, the median settles the sample into its case aggregate.
func (p *pipeline) landVote(job judgeJob, score *float64, detail string) {
	key := [4]int64{job.prep.run.ID, job.model.ID, job.c.ID, int64(job.sampleNo)}
	p.mu.Lock()
	sv, ok := p.votes[key]
	if !ok {
		// The sample settled already (duplicate or late vote after a
		// cancel): the row is recorded above; nothing more to fold.
		p.mu.Unlock()
		p.decrementPending(job)
		return
	}
	sv.scores[job.slot] = score
	sv.details = append(sv.details, voteDetail(job.judgeModel, score, detail))
	sv.landed++
	complete := sv.landed == sv.expected
	var median *float64
	var rule string
	if complete {
		median, rule = medianOfVotes(sv.scores)
		delete(p.votes, key)
	}
	p.mu.Unlock()

	if complete {
		d := fmt.Sprintf("sample %d/%d: %s → %s", job.sampleNo, job.expectedSamples, strings.Join(sv.details, ", "), rule)
		p.settleSample(job.prep.run.ID, job.model.ID, job.c.ID, median, d)
	}
	p.decrementPending(job)
}

// decrementPending drops one judge job from its run's outstanding count
// and finishes the run when nothing is left anywhere.
func (p *pipeline) decrementPending(job judgeJob) {
	p.mu.Lock()
	p.judgePending[job.prep.run.ID]--
	finished := p.runFinishedLocked(job.prep.run.ID)
	p.mu.Unlock()
	if finished {
		p.finishRun(p.preps[job.prep.run.ID])
	}
}

// markCellsDone records that a run's exam cells are exhausted; the run
// finishes once its judge queue is also empty.
func (p *pipeline) markCellsDone(runID int64) {
	p.mu.Lock()
	finished := p.cellsDoneLocked(runID)
	p.mu.Unlock()
	if finished {
		p.finishRun(p.preps[runID])
	}
}

func (p *pipeline) cellsDoneLocked(runID int64) bool {
	p.cellsDone[runID] = true
	return p.runFinishedLocked(runID)
}

func (p *pipeline) runFinishedLocked(runID int64) bool {
	if p.finished[runID] || !p.cellsDone[runID] || p.judgePending[runID] != 0 {
		return false
	}
	p.finished[runID] = true
	return true
}

// isFinished reports whether the run completed both stages.
func (p *pipeline) isFinished(runID int64) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.finished[runID]
}

// setCellsTotal records how many exam cells the batch will run.
func (p *pipeline) setCellsTotal(n int) {
	p.mu.Lock()
	p.examTotal = n
	p.mu.Unlock()
}

// noteCellStart / noteCellDone track the cell pool's occupancy.
func (p *pipeline) noteCellStart() {
	p.mu.Lock()
	p.examStarted++
	p.mu.Unlock()
}

func (p *pipeline) noteCellDone() {
	p.mu.Lock()
	p.examDone++
	p.mu.Unlock()
}

// modelQueueDepth is one subject's judge-stage progress (GH #179).
type modelQueueDepth struct {
	ModelDBID  int64
	JudgeDone  int
	JudgeTotal int
}

// queueDepth snapshots the live queue state of both stages (GH #178) plus
// the per-model judge progress (GH #179).
func (p *pipeline) queueDepth() (examPending, examInflight, judgePending, judgeInflight int, perModel []modelQueueDepth) {
	p.mu.Lock()
	defer p.mu.Unlock()
	examPending = p.examTotal - p.examStarted
	examInflight = p.examStarted - p.examDone
	judgePending = p.judgeOffered - p.judgeTaken
	judgeInflight = p.judgeTaken - p.judgeDone
	for modelID, total := range p.judgeOfferedByModel {
		perModel = append(perModel, modelQueueDepth{
			ModelDBID:  modelID,
			JudgeDone:  p.judgeDoneByModel[modelID],
			JudgeTotal: total,
		})
	}
	sort.Slice(perModel, func(a, b int) bool { return perModel[a].ModelDBID < perModel[b].ModelDBID })
	return examPending, examInflight, judgePending, judgeInflight, perModel
}

// setCellsTotal is wired by the batch executor; the retry path leaves the
// exam counters at zero (its report surface is the progress grid).

// costFor returns (creating on first use) the run's cost accumulator.
func (p *pipeline) costFor(runID int64) *costState {
	cs, ok := p.costs[runID]
	if !ok {
		cs = &costState{}
		p.costs[runID] = cs
	}
	return cs
}

// callCostUSD prices one call against the registry: nil when the model's
// price is unregistered or the hub reported no token usage.
func (p *pipeline) callCostUSD(modelID string, inTok, outTok *int) *float64 {
	info := registry.Lookup(modelID, p.overrides)
	if info.PriceIn == nil || info.PriceOut == nil || inTok == nil || outTok == nil {
		return nil
	}
	cost := (float64(*inTok)**info.PriceIn + float64(*outTok)**info.PriceOut) / 1e6
	return &cost
}

// recordExamCost folds one answer call's cost into the run.
func (p *pipeline) recordExamCost(runID int64, modelID string, inTok, outTok *int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	cs := p.costFor(runID)
	if cost := p.callCostUSD(modelID, inTok, outTok); cost != nil {
		cs.exam += *cost
	} else {
		cs.unknown = true
	}
}

// recordJudgeCost folds one jury call's cost into the run.
func (p *pipeline) recordJudgeCost(runID int64, judgeModel string, inTok, outTok *int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	cs := p.costFor(runID)
	if cost := p.callCostUSD(judgeModel, inTok, outTok); cost != nil {
		cs.judge += *cost
	} else {
		cs.unknown = true
	}
}

// flushCosts persists every run's accumulated estimate: NULL when any
// component was unpriceable (GH #178), else the exam/judge split.
func (p *pipeline) flushCosts() {
	p.mu.Lock()
	costs := make(map[int64]costState, len(p.costs))
	for runID, cs := range p.costs {
		costs[runID] = *cs
	}
	p.mu.Unlock()
	for runID, cs := range costs {
		if cs.unknown {
			if err := p.e.db.SetEvalRunCost(runID, nil, nil); err != nil {
				slog.Error("evaluator: null estimated cost", "run_id", runID, "error", err)
			}
			continue
		}
		if err := p.e.db.SetEvalRunCost(runID, &cs.exam, &cs.judge); err != nil {
			slog.Error("evaluator: persist estimated cost", "run_id", runID, "error", err)
		}
	}
}

// voteDetail renders one judge's vote for the case detail string, keeping
// the judge's own reason (or failure cause) visible.
func voteDetail(judgeModel string, score *float64, detail string) string {
	if score == nil {
		if detail == "" {
			return judgeModel + "=FAIL"
		}
		return fmt.Sprintf("%s=FAIL(%s)", judgeModel, detail)
	}
	if detail == "" {
		return fmt.Sprintf("%s=%.2f", judgeModel, *score)
	}
	return fmt.Sprintf("%s=%.2f (%s)", judgeModel, *score, detail)
}

// medianOfVotes applies the partial-jury rule (ADR 0016): three votes take
// the middle, two the mean, one itself, zero a null (W7 — failed judges
// never count as zero). The detail names the rule applied.
func medianOfVotes(scores []*float64) (*float64, string) {
	var vals []float64
	for _, s := range scores {
		if s != nil {
			vals = append(vals, *s)
		}
	}
	if len(vals) == 0 {
		return nil, "unjudged (all judges failed)"
	}
	sort.Float64s(vals)
	switch len(vals) {
	case 1:
		v := vals[0]
		return &v, fmt.Sprintf("median %.2f (1 vote)", v)
	case 2:
		v := (vals[0] + vals[1]) / 2
		return &v, fmt.Sprintf("median %.2f (2 votes, mean)", v)
	default:
		v := vals[1]
		return &v, fmt.Sprintf("median %.2f (3 votes)", v)
	}
}
