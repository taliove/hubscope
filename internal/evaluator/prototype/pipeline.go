package main

// PROTOTYPE — throwaway, not production code. Do not import.
//
// Question being answered: does the proposed eval-revamp state model hold up?
//
//	one-click trigger
//	  -> PROBE every candidate (reachable? speed? stability over k rounds?)
//	  -> gate: abort when the subject itself is unreachable
//	  -> JURY selection: 3 judges picked by policy (balanced/speed/iq/cost)
//	     from a built-in registry of model characteristics + pricing
//	  -> RUN as a decoupled 3-stage async pipeline with separate worker
//	     pools: exam (answer) -> judge x3 (per-judge scores) -> median
//	  -> final score = median of the jury; per-judge scores + spread shown
//
// Open design rules this sim makes concrete (feel them, then decide):
//   - partial jury: median of 2 = mean of 2; of 1 = itself; of 0 = unscored
//   - the subject is excluded from its own jury when enough alternatives
//     exist (self-preference bias), flagged when it cannot be
//   - a failed judge call is never retried (W7); its slot scores null
//   - exam stage keeps today's per-model circuit breaker (5 consecutive)
//
// The TUI in main.go is a thin shell; this file is the pure, portable logic.
// Run: make proto-eval

import (
	"fmt"
	"math"
	"math/rand/v2"
	"sort"
)

const (
	tickMs       = 200 // virtual milliseconds per tick
	probeRounds  = 3   // stability samples per model during probe
	probeTokens  = 16  // output tokens per probe call
	promptTokens = 200 // assumed prompt size of an exam answer call
	answerTokens = 400 // assumed answer size of an exam call
	judgeInTok   = 600 // judge prompt: case + rubric + answer
	judgeOutTok  = 150 // judge verdict size
	examWorkers  = 2
	judgeWorkers = 2
	numCases     = 5
	numSamples   = 2
	circuitLimit = 5
)

// Policy is the jury-selection strategy.
type Policy int

const (
	PolicyBalanced Policy = iota
	PolicySpeed
	PolicyIQ
	PolicyCost
)

var policyNames = []string{"balanced", "speed", "iq", "cost"}

func (p Policy) String() string { return policyNames[p] }

// Model is one roster entry: the stand-in for the built-in registry of model
// characteristics (IQ tier, speed tier) and pricing the proposal wants.
type Model struct {
	ID       string
	IQ       float64 // 1..10 quality tier
	TPS      float64 // output tokens/sec at full health
	InPrice  float64 // USD per 1M input tokens
	OutPrice float64 // USD per 1M output tokens
	FailRate float64 // per-call failure probability
	BaseMs   int     // fixed per-call latency overhead
	Down     bool    // injected outage
}

// roster is the simulated hub's model list.
func roster() []*Model {
	return []*Model{
		{ID: "qwen3-235b", IQ: 9.0, TPS: 45, InPrice: 2.0, OutPrice: 8.0, FailRate: 0.02, BaseMs: 300},
		{ID: "deepseek-v3", IQ: 8.5, TPS: 60, InPrice: 0.8, OutPrice: 2.0, FailRate: 0.03, BaseMs: 250},
		{ID: "qwen3-30b-a3b", IQ: 7.5, TPS: 100, InPrice: 0.3, OutPrice: 1.0, FailRate: 0.02, BaseMs: 200},
		{ID: "gpt-4o-mini", IQ: 7.0, TPS: 90, InPrice: 0.15, OutPrice: 0.6, FailRate: 0.01, BaseMs: 200},
		{ID: "glm-4-flash", IQ: 6.0, TPS: 110, InPrice: 0.1, OutPrice: 0.1, FailRate: 0.04, BaseMs: 180},
		{ID: "llama-3.1-8b", IQ: 5.5, TPS: 120, InPrice: 0, OutPrice: 0, FailRate: 0.05, BaseMs: 150},
	}
}

// ProbeResult is what the pre-flight probe learns about one model.
type ProbeResult struct {
	Reachable  bool
	Rounds     int
	Successes  int
	AvgTPS     float64
	AvgLatency int
	Cost       float64
}

func (p *ProbeResult) SuccessRate() float64 {
	if p.Rounds == 0 {
		return 0
	}
	return float64(p.Successes) / float64(p.Rounds)
}

// Phase of the campaign state machine.
type Phase int

const (
	PhaseProbe Phase = iota
	PhaseJury        // checkpoint: jury selected, waiting for launch
	PhaseRun
	PhaseDone
)

var phaseNames = []string{"PROBE", "JURY-CHECKPOINT", "RUN", "DONE"}

func (p Phase) String() string { return phaseNames[p] }

type probeTask struct {
	model     int
	round     int
	ticksLeft int
	callMs    int
	res       *ProbeResult
}

type examJob struct {
	caseID, sample int
	ticksLeft      int
	callMs         int
}

type judgeJob struct {
	caseID, sample, slot int
	ticksLeft            int
}

type sampleState struct {
	answerOK bool
	settled  bool
	scores   [3]*float64
	done     [3]bool
	median   *float64
}

// Sim is the whole prototype state: pure logic, no I/O.
type Sim struct {
	rng        *rand.Rand
	seed       int64
	lastCallMs int
	models     []*Model
	subject    int
	policy     Policy
	phase      Phase
	tick       int
	diff       [numCases]float64 // deterministic per-case difficulty jitter
	jury       []int             // model indexes, len <= 3
	juryNote   string

	probes     map[int]*ProbeResult
	probeTasks []*probeTask

	examPending   []examJob
	examInflight  []*examJob
	judgePending  []judgeJob
	judgeInflight []*judgeJob

	grid      [numCases][numSamples]*sampleState
	caseScore [numCases]*float64

	examDone, examTotal   int
	judgeDone, judgeTotal int
	examCost              float64
	judgeCost             [3]float64
	answerTok             int
	answerMsSum           int
	circuit               int
	final                 *float64

	events []string
}

// NewSim builds a fresh simulation; subjectIdx defaults to the weakest model
// (evaluating a cheap model with a strong jury is the interesting case).
func NewSim(seed int64, subjectIdx int) *Sim {
	s := &Sim{
		rng:     rand.New(rand.NewPCG(uint64(seed), uint64(seed)^0x9e3779b9)),
		seed:    seed,
		models:  roster(),
		subject: subjectIdx,
		policy:  PolicyBalanced,
		probes:  map[int]*ProbeResult{},
	}
	for c := range s.diff {
		s.diff[c] = (s.rng.Float64() - 0.5) * 0.2
	}
	for c := range s.grid {
		for m := range s.grid[c] {
			s.grid[c][m] = &sampleState{}
		}
	}
	s.startProbe()
	return s
}

func (s *Sim) logf(format string, args ...any) {
	s.events = append(s.events, fmt.Sprintf("[t%04d] ", s.tick)+fmt.Sprintf(format, args...))
	if len(s.events) > 10 {
		s.events = s.events[len(s.events)-10:]
	}
}

// Tick advances the simulation one virtual 200ms step. The jury checkpoint
// is sticky: only Launch() (or a policy key) moves out of PhaseJury.
func (s *Sim) Tick() {
	s.tick++
	switch s.phase {
	case PhaseProbe:
		s.tickProbe()
	case PhaseRun:
		s.tickRun()
	}
}

// --- probe stage -----------------------------------------------------------

func (s *Sim) startProbe() {
	s.phase = PhaseProbe
	s.probeTasks = nil
	for i := range s.models {
		res := &ProbeResult{}
		s.probes[i] = res
		s.probeTasks = append(s.probeTasks, s.nextProbeCall(i, 0, res))
	}
	s.logf("probe started: %d models x %d rounds", len(s.models), probeRounds)
}

func (s *Sim) nextProbeCall(model, round int, res *ProbeResult) *probeTask {
	m := s.models[model]
	return &probeTask{model: model, round: round, res: res,
		ticksLeft: s.callTicks(m, probeTokens), callMs: s.lastCallMs}
}

// callTicks draws one call's duration in ticks; lastCallMs keeps the drawn
// milliseconds for TPS measurement.
func (s *Sim) callTicks(m *Model, outTokens int) int {
	ms := float64(m.BaseMs) + float64(outTokens)/m.TPS*1000
	ms *= 0.9 + 0.2*s.rng.Float64()
	s.lastCallMs = int(ms)
	t := int(math.Round(ms / tickMs))
	if t < 1 {
		t = 1
	}
	return t
}

func (s *Sim) callFails(m *Model) bool {
	return m.Down || s.rng.Float64() < m.FailRate
}

func callCost(m *Model, inTok, outTok int) float64 {
	return (float64(inTok)*m.InPrice + float64(outTok)*m.OutPrice) / 1e6
}

func (s *Sim) tickProbe() {
	alive := false
	for _, pt := range s.probeTasks {
		if pt == nil {
			continue
		}
		alive = true
		pt.ticksLeft--
		if pt.ticksLeft > 0 {
			continue
		}
		m := s.models[pt.model]
		pt.res.Rounds++
		pt.res.Cost += callCost(m, promptTokens, probeTokens)
		if s.callFails(m) {
			s.logf("probe %s round %d/%d: FAIL", m.ID, pt.round+1, probeRounds)
		} else {
			pt.res.Successes++
			pt.res.AvgTPS += float64(probeTokens) / (float64(pt.callMs) / 1000)
			pt.res.AvgLatency += pt.callMs
		}
		if pt.round+1 < probeRounds {
			s.probeTasks[pt.model] = s.nextProbeCall(pt.model, pt.round+1, pt.res)
		} else {
			if pt.res.Successes > 0 {
				pt.res.Reachable = true
				pt.res.AvgTPS /= float64(pt.res.Successes)
				pt.res.AvgLatency /= pt.res.Successes
			}
			s.probeTasks[pt.model] = nil
			s.logf("probe %s done: reachable=%v succ=%d/%d tps=%.0f",
				m.ID, pt.res.Reachable, pt.res.Successes, pt.res.Rounds, pt.res.AvgTPS)
		}
	}
	if alive {
		return
	}
	// Probe finished: the gate.
	if !s.probes[s.subject].Reachable {
		s.phase = PhaseDone
		s.logf("ABORTED by probe gate: subject %s unreachable — no exam burned", s.models[s.subject].ID)
		return
	}
	s.selectJury()
	s.phase = PhaseJury
	s.logf("jury checkpoint: %s (policy %s) — press space to launch", s.juryNote, s.policy)
}

// --- jury selection --------------------------------------------------------

type juryCand struct {
	idx                 int
	score, iq, spd, chp float64
}

// selectJury ranks reachable models by the active policy and takes the top 3.
// The subject is excluded when at least 3 other reachable models exist;
// otherwise it serves with a self-judge warning.
func (s *Sim) selectJury() {
	maxPrice := 0.0
	for i, m := range s.models {
		if p := s.probes[i]; p == nil || !p.Reachable {
			continue
		}
		if price := m.InPrice + m.OutPrice; price > maxPrice {
			maxPrice = price
		}
	}
	rank := func(excludeSubject bool) []juryCand {
		var cands []juryCand
		for i, m := range s.models {
			p := s.probes[i]
			if p == nil || !p.Reachable || (excludeSubject && i == s.subject) {
				continue
			}
			iq := m.IQ / 10
			spd := m.TPS / 120
			chp := 1.0
			if maxPrice > 0 {
				chp = 1 - (m.InPrice+m.OutPrice)/maxPrice
			}
			var score float64
			switch s.policy {
			case PolicySpeed:
				score = 0.1*iq + 0.7*spd + 0.2*chp
			case PolicyIQ:
				score = 0.8*iq + 0.1*spd + 0.1*chp
			case PolicyCost:
				score = 0.15*iq + 0.15*spd + 0.7*chp
			default:
				score = 0.4*iq + 0.3*spd + 0.3*chp
			}
			cands = append(cands, juryCand{idx: i, score: score, iq: iq, spd: spd, chp: chp})
		}
		sort.Slice(cands, func(a, b int) bool {
			if cands[a].score != cands[b].score {
				return cands[a].score > cands[b].score
			}
			return cands[a].idx < cands[b].idx
		})
		return cands
	}

	cands := rank(true)
	note := ""
	if len(cands) < 3 {
		cands = rank(false)
		note = "WARNING: subject serves on its own jury (self-preference bias)"
	}
	s.jury = s.jury[:0]
	for i := 0; i < len(cands) && i < 3; i++ {
		s.jury = append(s.jury, cands[i].idx)
	}
	if len(s.jury) < 3 {
		note += fmt.Sprintf(" SHORT JURY: only %d reachable judge(s)", len(s.jury))
	}
	names := ""
	for i, j := range s.jury {
		if i > 0 {
			names += ", "
		}
		names += s.models[j].ID
	}
	if note != "" {
		note = " —" + note
	}
	s.juryNote = fmt.Sprintf("[%s]%s", names, note)
}

// SetPolicy re-selects the jury; only legal at the jury checkpoint.
func (s *Sim) SetPolicy(p Policy) {
	s.policy = p
	if s.phase == PhaseJury {
		s.selectJury()
		s.logf("policy -> %s: %s", p, s.juryNote)
	}
}

// --- run stage: exam / judge / score, decoupled worker pools ---------------

// Launch moves from the jury checkpoint into the run.
func (s *Sim) Launch() {
	if s.phase != PhaseJury {
		return
	}
	s.phase = PhaseRun
	for c := 0; c < numCases; c++ {
		for m := 0; m < numSamples; m++ {
			s.examPending = append(s.examPending, examJob{caseID: c, sample: m})
		}
	}
	s.examTotal = len(s.examPending)
	s.logf("run launched: %d cases x %d samples, jury=%d", numCases, numSamples, len(s.jury))
}

func (s *Sim) tickRun() {
	subj := s.models[s.subject]
	// Refill exam workers.
	for len(s.examInflight) < examWorkers && len(s.examPending) > 0 {
		j := s.examPending[0]
		s.examPending = s.examPending[1:]
		j.ticksLeft = s.callTicks(subj, answerTokens)
		j.callMs = s.lastCallMs
		s.examInflight = append(s.examInflight, &j)
	}
	// Refill judge workers.
	for len(s.judgeInflight) < judgeWorkers && len(s.judgePending) > 0 {
		j := s.judgePending[0]
		s.judgePending = s.judgePending[1:]
		j.ticksLeft = s.callTicks(s.models[s.jury[j.slot]], judgeOutTok)
		s.judgeInflight = append(s.judgeInflight, &j)
	}
	// Advance exams.
	kept := s.examInflight[:0]
	for _, j := range s.examInflight {
		j.ticksLeft--
		if j.ticksLeft > 0 {
			kept = append(kept, j)
			continue
		}
		s.finishExam(j)
	}
	s.examInflight = kept
	// Advance judgings.
	keptJ := s.judgeInflight[:0]
	for _, j := range s.judgeInflight {
		j.ticksLeft--
		if j.ticksLeft > 0 {
			keptJ = append(keptJ, j)
			continue
		}
		s.finishJudge(j)
	}
	s.judgeInflight = keptJ

	if s.examDone == s.examTotal && s.judgeDone == s.judgeTotal {
		s.settleRun()
	}
}

func (s *Sim) finishExam(j *examJob) {
	s.examDone++
	subj := s.models[s.subject]
	smp := s.grid[j.caseID][j.sample]
	if s.callFails(subj) {
		smp.settled = true // no answer -> never judged (today's convention)
		s.circuit++
		s.logf("exam FAIL case %d sample %d (circuit %d/%d)", j.caseID+1, j.sample+1, s.circuit, circuitLimit)
		if s.circuit >= circuitLimit {
			dropped := len(s.examPending) + len(s.examInflight)
			for _, d := range s.examPending {
				s.grid[d.caseID][d.sample].settled = true
			}
			for _, d := range s.examInflight {
				s.grid[d.caseID][d.sample].settled = true
			}
			s.examDone += dropped
			s.examPending = nil
			s.examInflight = nil
			s.logf("CIRCUIT OPEN: dropped %d pending exams", dropped)
		}
		s.settleCase(j.caseID)
		return
	}
	s.circuit = 0
	smp.answerOK = true
	s.answerTok += answerTokens
	s.answerMsSum += j.callMs
	s.examCost += callCost(subj, promptTokens, answerTokens)
	if len(s.jury) == 0 {
		smp.settled = true
		s.settleCase(j.caseID)
		return
	}
	for slot := range s.jury {
		s.judgePending = append(s.judgePending, judgeJob{caseID: j.caseID, sample: j.sample, slot: slot})
		s.judgeTotal++
	}
}

func (s *Sim) finishJudge(j *judgeJob) {
	s.judgeDone++
	jm := s.models[s.jury[j.slot]]
	smp := s.grid[j.caseID][j.sample]
	smp.done[j.slot] = true
	if s.callFails(jm) {
		s.logf("judge %s FAILED case %d sample %d (slot stays null, W7)", jm.ID, j.caseID+1, j.sample+1)
	} else {
		s.judgeCost[j.slot] += callCost(jm, judgeInTok, judgeOutTok)
		score := s.judgeScore(j, jm)
		smp.scores[j.slot] = &score
	}
	// Sample settles when every jury slot reported (null counts).
	for slot := range s.jury {
		if !smp.done[slot] {
			return
		}
	}
	var vals []float64
	for slot := range s.jury {
		if smp.scores[slot] != nil {
			vals = append(vals, *smp.scores[slot])
		}
	}
	if len(vals) > 0 {
		med := medianOf(vals)
		smp.median = &med
	}
	smp.settled = true
	s.settleCase(j.caseID)
}

// judgeScore simulates one judge's verdict: subject true quality + per-case
// difficulty + judge noise + self-preference bias when judging oneself.
func (s *Sim) judgeScore(j *judgeJob, jm *Model) float64 {
	trueQ := s.models[s.subject].IQ/10 + s.diff[j.caseID]
	noise := (s.rng.Float64() - 0.5) * 0.16
	bias := 0.0
	if s.jury[j.slot] == s.subject {
		bias = 0.08
	}
	return math.Max(0, math.Min(1, trueQ+noise+bias))
}

// medianOf is the partial-jury rule under test: 3 -> middle, 2 -> mean,
// 1 -> itself. Caller guarantees len(vals) >= 1.
func medianOf(vals []float64) float64 {
	sort.Float64s(vals)
	switch len(vals) {
	case 1:
		return vals[0]
	case 2:
		return (vals[0] + vals[1]) / 2
	default:
		return vals[1]
	}
}

func (s *Sim) settleCase(caseID int) {
	var sum float64
	var n int
	for m := 0; m < numSamples; m++ {
		smp := s.grid[caseID][m]
		if !smp.settled {
			return
		}
		if smp.median != nil {
			sum += *smp.median
			n++
		}
	}
	if n > 0 {
		avg := sum / float64(n)
		s.caseScore[caseID] = &avg
	}
}

func (s *Sim) settleRun() {
	var sum float64
	var n int
	for c := 0; c < numCases; c++ {
		if s.caseScore[c] != nil {
			sum += *s.caseScore[c]
			n++
		}
	}
	if n > 0 {
		avg := sum / float64(n)
		s.final = &avg
	}
	s.phase = PhaseDone
	s.logf("RUN DONE: final=%s (%d/%d cases scored) exam=$%.4f judge=$%.4f",
		s.FinalString(), n, numCases, s.examCost, s.TotalJudgeCost())
}

// FinalString renders the campaign's final score.
func (s *Sim) FinalString() string {
	if s.final == nil {
		return "unscored"
	}
	return fmt.Sprintf("%.3f", *s.final)
}

// TotalJudgeCost sums the per-judge cost.
func (s *Sim) TotalJudgeCost() float64 {
	return s.judgeCost[0] + s.judgeCost[1] + s.judgeCost[2]
}

// KillModel forces an outage; in-flight calls fail on completion.
func (s *Sim) KillModel(i int) {
	s.models[i].Down = true
	s.logf("KILLED %s (in-flight calls will fail)", s.models[i].ID)
}

// ReviveAll clears every outage.
func (s *Sim) ReviveAll() {
	for _, m := range s.models {
		m.Down = false
	}
	s.logf("all models revived")
}

// --- display helpers (pure formatting over state, used by the TUI) ---------

// Spread reports the max-min disagreement across a sample's judge scores.
func (smp *sampleState) spread() float64 {
	lo, hi := 1.0, 0.0
	n := 0
	for _, sc := range smp.scores {
		if sc == nil {
			continue
		}
		n++
		if *sc < lo {
			lo = *sc
		}
		if *sc > hi {
			hi = *sc
		}
	}
	if n < 2 {
		return 0
	}
	return hi - lo
}
