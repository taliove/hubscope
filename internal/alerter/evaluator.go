package alerter

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/taliove/hubscope/internal/scheduler"
	"github.com/taliove/hubscope/internal/status"
	"github.com/taliove/hubscope/internal/store"
)

// Evaluator decides when probe results warrant a Lark alert. It is invoked
// once per finished probe round (manual or scheduled) via HandleRound.
//
// Alerting rules:
//   - failures reach status.DownThreshold and the endpoint is not yet alerted →
//     record one "down" event and buffer the transition into the aggregation
//     window;
//   - the outage continues → stay silent (no repeat alerts);
//   - the endpoint was alerted and a success arrives → record one
//     "recovered" event and buffer the transition.
//
// Sending is decoupled from the decision (spec 0017, ADR 0014): the event
// lands in alert_events at decision time with sent_ok=false ("delivery
// unconfirmed") and the alerted flag flips immediately, so the W5 lazy
// state rebuild never depends on a send having happened; the buffered
// transitions flush 60s later as one aggregated card per kind, which writes
// the events' sent_ok back and records a batch event with the actual text.
//
// The per-endpoint alerted flag lives in memory; when an endpoint has no
// cached state it is rebuilt from alert_events (a trailing "down" event means
// the outage was already reported), so a restart does not re-alert.
type Evaluator struct {
	db     *store.DB
	sender *LarkSender
	clock  scheduler.Clock

	// mu serializes evaluation per process. HandleRound/HandleCampaign and
	// window-flush sends happen under the lock; they are rare (transitions
	// only) and bounded by the sender timeout, and the store is
	// single-connection anyway, so a global lock is the simplest correct
	// choice. The login brute-force path (login_alert.go) sends off-lock
	// instead: it has no alerted state to protect and must never block the
	// login request path.
	mu      sync.Mutex
	alerted map[int64]bool

	// Vendor group alerts (spec 0017 ticket 3, GH #66): the group-open flag
	// per family, lazily rebuilt from persisted group events after a restart
	// (the group counterpart of alerted). Evaluated at every endpoint
	// transition's decision point in group.go.
	groupAlerted map[string]bool

	// Alert aggregation window (spec 0017): transitions buffer here at
	// decision time and flush as one aggregated card per kind when the
	// timer fires. A process restart drops the buffer but never the events,
	// so nothing is re-reported. groupPending holds the buffered group
	// transitions (group_down / group_recovered) sharing the same window.
	// windowFiresAt is the scheduled fire time of the armed window — the
	// quiet gate evaluates it rather than the handler's run time, so a
	// flush executed late (fake-clock jumps in tests) still honors the
	// quiet state of the moment the window was due.
	pending       []bufferedTransition
	groupPending  []bufferedGroupTransition
	windowTimer   scheduler.Timer
	windowFiresAt time.Time

	// Quiet hours (spec 0017 ticket 4, GH #67): the single boundary timer
	// fires at the next window edge (start or end); quietBoundaryAt is what
	// it is armed for. quietDone is closed when the armed timer is
	// superseded or disarmed, releasing its waiter goroutine — invariant:
	// quietTimer != nil ⟺ quietDone != nil. quietScoreDrops holds the
	// score_drop events deferred inside the window, riding the
	// end-of-window summary.
	quietTimer      scheduler.Timer
	quietBoundaryAt time.Time
	quietDone       chan struct{}
	quietScoreDrops []quietScoreDrop
}

// NewEvaluator creates an Evaluator persisting events through db and sending
// via sender. The aggregation window runs on the real clock; tests swap in a
// fake one via UseClock.
func NewEvaluator(db *store.DB, sender *LarkSender) *Evaluator {
	return &Evaluator{
		db:           db,
		sender:       sender,
		clock:        scheduler.RealClock{},
		alerted:      make(map[int64]bool),
		groupAlerted: make(map[string]bool),
	}
}

// UseClock swaps the clock driving the aggregation window (W4). It is a
// construction-time seam (the server's WithAlertClock option applies it
// before the server serves traffic) and is not safe against concurrent
// rounds.
func (e *Evaluator) UseClock(clock scheduler.Clock) {
	e.clock = clock
}

// HandleRound evaluates one finished probe round for alerting. It never
// fails the round: every problem is logged instead of returned.
func (e *Evaluator) HandleRound(ctx context.Context, endpointID int64, _ []store.Probe) {
	e.mu.Lock()
	defer e.mu.Unlock()

	consecutive, err := e.db.CountConsecutiveFailures(endpointID)
	if err != nil {
		slog.Error("alerter: count consecutive failures", "endpoint_id", endpointID, "error", err)
		return
	}
	alerted, err := e.isAlerted(endpointID)
	if err != nil {
		slog.Error("alerter: rebuild alert state", "endpoint_id", endpointID, "error", err)
		return
	}

	switch {
	case !alerted && consecutive >= status.DownThreshold:
		e.transition(endpointID, store.AlertKindDown, true)
	case alerted && consecutive < status.DownThreshold:
		// A success since the down alert ended the outage (consecutive
		// failures reset below the threshold).
		e.transition(endpointID, store.AlertKindRecovered, false)
	}
}

// isAlerted reports whether the endpoint currently has an open down alert,
// lazily rebuilding the in-memory flag from persisted events on first sight.
// The evaluator is the only writer of down/recovered events, so caching a
// negative answer is safe.
func (e *Evaluator) isAlerted(endpointID int64) (bool, error) {
	if flagged, ok := e.alerted[endpointID]; ok {
		return flagged, nil
	}
	latest, err := e.db.LatestDownRecoveryEvent(endpointID)
	if err != nil {
		return false, err
	}
	flagged := latest != nil && latest.Kind == store.AlertKindDown
	e.alerted[endpointID] = flagged
	return flagged, nil
}

// transition records the alert event at decision time, flips the alerted
// flag, evaluates the endpoint's vendor group at the same decision point
// (spec 0017 ticket 3: the group share is computed from the just-flipped
// flags and the just-recorded event, so endpoint and group transitions
// share one consistent snapshot), and buffers the transition into the
// aggregation window (ADR 0014). The event lands with sent_ok=false —
// "delivery unconfirmed" — and the window flush later sends one aggregated
// card per kind, writes the real results back, and records a batch event
// with the actual text sent.
//
// The endpoint transition is marked absorbed when its group is open before
// or opens/closes with this very transition: absorbed transitions never
// render into endpoint cards — their story is told by the group cards —
// and their events honestly stay sent_ok=false (never individually
// delivered).
//
// The flag flips even when the webhook is unconfigured or alerts are
// disabled: the outage counts as reported and is not retried (W5); an
// unconfigured webhook still records no event at all. Group evaluation runs
// either way — the group flag makes the same trade inside
// groupTransitionLocked.
func (e *Evaluator) transition(endpointID int64, kind string, alerted bool) {
	alert, err := e.buildEndpointAlert(endpointID, kind)
	if err != nil {
		slog.Error("alerter: build alert content", "kind", kind, "endpoint_id", endpointID, "error", err)
		return
	}
	e.alerted[endpointID] = alerted

	webhook, err := e.db.GetSetting(store.SettingLarkWebhookURL, "")
	if err != nil {
		slog.Error("alerter: read webhook setting", "error", err)
		return
	}
	enabled, err := e.db.GetSettingBool(store.SettingAlertEnabled, store.DefaultAlertEnabled)
	if err != nil {
		slog.Error("alerter: read alert_enabled setting", "error", err)
		return
	}
	configured := webhook != "" && enabled

	var eventID int64
	if configured {
		event, err := e.db.CreateAlertEvent(store.AlertEvent{
			EndpointID: &endpointID,
			Kind:       kind,
			Message:    alert.text,
			SentOK:     false, // delivery unconfirmed until the window flush
		})
		if err != nil {
			slog.Error("alerter: record alert event", "kind", kind, "endpoint_id", endpointID, "error", err)
			return
		}
		eventID = event.ID
	} else {
		// Not configured: the transition happened but no event is recorded.
		slog.Debug("alerter: alert skipped (webhook not configured or alerts disabled)", "kind", kind, "endpoint_id", endpointID)
	}

	groupWasOpen, err := e.isGroupAlerted(alert.family)
	if err != nil {
		slog.Error("alerter: rebuild group alert state", "family", alert.family, "error", err)
		return
	}
	e.evaluateGroupLocked(alert.family)

	if !configured {
		return
	}
	groupOpenNow, err := e.isGroupAlerted(alert.family)
	if err != nil {
		slog.Error("alerter: rebuild group alert state", "family", alert.family, "error", err)
		return
	}
	e.bufferLocked(bufferedTransition{
		eventID:  eventID,
		kind:     kind,
		alert:    alert,
		absorbed: groupWasOpen || groupOpenNow,
	})
}

// buildEndpointAlert composes the per-endpoint alert content — hub name,
// model ID, protocol, family (the group-alert dimension), and the latest
// error summary for down alerts — frozen at decision time so the window
// flush can render the endpoint inside an aggregated card even if the
// endpoint changes before the flush. The plain text is persisted on the
// per-endpoint event (the alert history table renders it unchanged).
func (e *Evaluator) buildEndpointAlert(endpointID int64, kind string) (endpointAlert, error) {
	endpoint, err := e.db.GetEndpoint(endpointID)
	if err != nil {
		return endpointAlert{}, fmt.Errorf("load endpoint: %w", err)
	}
	model, err := e.db.GetModel(endpoint.ModelID)
	if err != nil {
		return endpointAlert{}, fmt.Errorf("load model: %w", err)
	}
	hub, err := e.db.GetHub(model.HubID)
	if err != nil {
		return endpointAlert{}, fmt.Errorf("load hub: %w", err)
	}

	alert := endpointAlert{
		hubName:  hub.Name,
		modelID:  model.ModelID,
		protocol: endpoint.Protocol,
		family:   model.Family,
	}
	if kind == store.AlertKindRecovered {
		alert.text = fmt.Sprintf("【HubScope】端点恢复:模型 %s(%s)已恢复正常。",
			model.ModelID, endpoint.Protocol)
		return alert, nil
	}

	alert.lastError = "未知错误"
	if latest, err := e.db.LatestProbe(endpointID); err == nil &&
		latest != nil && !latest.OK && latest.ErrorSummary != nil {
		alert.lastError = *latest.ErrorSummary
	}
	alert.text = fmt.Sprintf("【HubScope】端点告警:模型 %s(%s)已连续 %d 次探测失败,最近错误:%s",
		model.ModelID, endpoint.Protocol, status.DownThreshold, alert.lastError)
	return alert, nil
}
