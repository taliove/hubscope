package scheduler

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/taliove2009/hubscope/internal/prober"
	"github.com/taliove2009/hubscope/internal/store"
)

// defaultIntervalSeconds is the global default probe interval.
const defaultIntervalSeconds = 300

// defaultMaxConcurrent caps how many endpoint rounds run in parallel.
const defaultMaxConcurrent = 8

// defaultPollInterval bounds how often the store is re-read for changes.
const defaultPollInterval = 5 * time.Second

// endpointState tracks the scheduling state of one endpoint.
type endpointState struct {
	running     bool
	hasRun      bool
	completedAt time.Time // clock time of the last completed round
	dropped     bool      // disabled while a round was in flight
}

// Scheduler periodically runs probe rounds for all enabled endpoints.
// Scheduling semantics:
//   - Every enabled endpoint is due immediately at startup.
//   - After a round completes, the next round is scheduled interval seconds
//     after the completion time (per-endpoint override or global default).
//   - An endpoint never re-enters while its previous round is in flight.
//   - Rounds across endpoints run concurrently, capped by a semaphore.
//   - The store is re-read on every tick, so PATCH changes (enable/disable,
//     interval override) apply to the next round.
type Scheduler struct {
	db              *store.DB
	prober          *prober.Prober
	clock           Clock
	defaultInterval time.Duration
	pollInterval    time.Duration

	sem     chan struct{}
	workers sync.WaitGroup
	wake    chan struct{}

	mu      sync.Mutex
	entries map[int64]*endpointState
}

// Option customizes a Scheduler.
type Option func(*Scheduler)

// WithDefaultInterval overrides the global default probe interval.
func WithDefaultInterval(d time.Duration) Option {
	return func(s *Scheduler) { s.defaultInterval = d }
}

// WithMaxConcurrent overrides the global concurrency cap.
func WithMaxConcurrent(n int) Option {
	return func(s *Scheduler) { s.sem = make(chan struct{}, n) }
}

// WithPollInterval overrides how often the store is re-read at most.
func WithPollInterval(d time.Duration) Option {
	return func(s *Scheduler) { s.pollInterval = d }
}

// New creates a Scheduler over the given store and prober, driven by clock.
func New(db *store.DB, p *prober.Prober, clock Clock, opts ...Option) *Scheduler {
	s := &Scheduler{
		db:              db,
		prober:          p,
		clock:           clock,
		defaultInterval: defaultIntervalSeconds * time.Second,
		pollInterval:    defaultPollInterval,
		sem:             make(chan struct{}, defaultMaxConcurrent),
		wake:            make(chan struct{}, 1),
		entries:         make(map[int64]*endpointState),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Run drives the scheduling loop until ctx is canceled. It stops dispatching
// new work on cancellation and blocks until in-flight workers finish, so it
// is safe to wait on for graceful shutdown.
func (s *Scheduler) Run(ctx context.Context) {
	defer s.workers.Wait()
	var timer Timer
	defer func() {
		if timer != nil {
			timer.Stop()
		}
	}()

	for {
		nextWake := s.dispatch(ctx)
		if timer != nil {
			timer.Stop()
		}
		timer = s.clock.NewTimer(nextWake.Sub(s.clock.Now()))
		timerC := timer.C()

		select {
		case <-ctx.Done():
			return
		case <-timerC:
		case <-s.wake:
		}
	}
}

// dispatch re-reads the enabled endpoint list, starts rounds that are due,
// forgets endpoints that were disabled, and returns when the loop should
// wake next.
func (s *Scheduler) dispatch(ctx context.Context) time.Time {
	now := s.clock.Now()
	next := now.Add(s.pollInterval)

	endpoints, err := s.db.ListEnabledEndpoints()
	if err != nil {
		slog.Error("scheduler: list enabled endpoints", "error", err)
		return next
	}
	if ctx.Err() != nil {
		return next
	}

	active := make(map[int64]bool, len(endpoints))
	for _, ep := range endpoints {
		active[ep.ID] = true
		interval := s.intervalFor(ep)

		s.mu.Lock()
		e, ok := s.entries[ep.ID]
		if !ok {
			e = &endpointState{}
			s.entries[ep.ID] = e
		}
		switch {
		case e.running:
			// Never re-enter an endpoint while a round is in flight.
		case !e.hasRun || !e.completedAt.Add(interval).After(now):
			e.running = true
			e.hasRun = true
			s.workers.Add(1)
			go s.runEndpoint(ctx, ep.ID)
		default:
			if due := e.completedAt.Add(interval); due.Before(next) {
				next = due
			}
		}
		s.mu.Unlock()
	}

	// Forget endpoints that were disabled or removed. A round already in
	// flight is left to finish; once it does, finish forgets the endpoint so
	// a later re-enable starts fresh and becomes due immediately.
	s.mu.Lock()
	for id, e := range s.entries {
		if !active[id] {
			if e.running {
				e.dropped = true
			} else {
				delete(s.entries, id)
			}
		}
	}
	s.mu.Unlock()

	return next
}

// runEndpoint executes one probe round under the concurrency semaphore.
func (s *Scheduler) runEndpoint(ctx context.Context, endpointID int64) {
	defer s.workers.Done()

	select {
	case s.sem <- struct{}{}:
		defer func() { <-s.sem }()
	case <-ctx.Done():
		s.finish(endpointID, false)
		return
	}

	// In-flight rounds run to completion even after ctx cancellation so
	// graceful shutdown waits for them instead of aborting mid-request.
	_, _ = s.prober.RunRound(context.WithoutCancel(ctx), endpointID)

	s.finish(endpointID, true)

	// Wake the loop so it can schedule the next round promptly.
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// finish records round completion under the scheduler lock. An endpoint
// dropped while its round was in flight is forgotten entirely, so a
// re-enable treats it as new (due immediately) instead of anchoring the
// next round to this completion.
func (s *Scheduler) finish(endpointID int64, completed bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.entries[endpointID]
	if !ok {
		return
	}
	if e.dropped {
		delete(s.entries, endpointID)
		return
	}
	e.running = false
	if completed {
		e.completedAt = s.clock.Now()
	}
}

// intervalFor resolves the effective interval for an endpoint: the
// per-endpoint override when set, otherwise the global default.
func (s *Scheduler) intervalFor(ep store.Endpoint) time.Duration {
	if ep.IntervalSeconds != nil && *ep.IntervalSeconds > 0 {
		return time.Duration(*ep.IntervalSeconds) * time.Second
	}
	return s.defaultInterval
}
