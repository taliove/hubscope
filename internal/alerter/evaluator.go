package alerter

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/taliove/hubscope/internal/status"
	"github.com/taliove/hubscope/internal/store"
)

// Evaluator decides when probe results warrant a Lark alert. It is invoked
// once per finished probe round (manual or scheduled) via HandleRound.
//
// Alerting rules:
//   - failures reach status.DownThreshold and the endpoint is not yet alerted →
//     send one "down" alert and record the event;
//   - the outage continues → stay silent (no repeat alerts);
//   - the endpoint was alerted and a success arrives → send one "recovered"
//     notice and record the event.
//
// The per-endpoint alerted flag lives in memory; when an endpoint has no
// cached state it is rebuilt from alert_events (a trailing "down" event means
// the outage was already reported), so a restart does not re-alert.
type Evaluator struct {
	db     *store.DB
	sender *LarkSender

	// mu serializes evaluation per process. HandleRound/HandleCampaign
	// sends happen under the lock; they are rare (transitions only) and
	// bounded by the sender timeout, and the store is single-connection
	// anyway, so a global lock is the simplest correct choice. The login
	// brute-force path (login_alert.go) sends off-lock instead: it has no
	// alerted state to protect and must never block the login request path.
	mu      sync.Mutex
	alerted map[int64]bool
}

// NewEvaluator creates an Evaluator persisting events through db and sending
// via sender.
func NewEvaluator(db *store.DB, sender *LarkSender) *Evaluator {
	return &Evaluator{
		db:      db,
		sender:  sender,
		alerted: make(map[int64]bool),
	}
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
		e.transition(ctx, endpointID, store.AlertKindDown, true)
	case alerted && consecutive < status.DownThreshold:
		// A success since the down alert ended the outage (consecutive
		// failures reset below the threshold).
		e.transition(ctx, endpointID, store.AlertKindRecovered, false)
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

// transition sends the alert (when configured), records the event, and
// updates the alerted flag. The flag flips even when the webhook is
// unconfigured or the send fails: an unconfigured webhook produces no event
// at all, while a failed send produces one with sent_ok=false — in both
// cases the outage counts as reported and is not retried.
func (e *Evaluator) transition(ctx context.Context, endpointID int64, kind string, alerted bool) {
	message, err := e.buildMessage(endpointID, kind)
	if err != nil {
		slog.Error("alerter: build alert message", "kind", kind, "endpoint_id", endpointID, "error", err)
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
	if webhook == "" || !enabled {
		// Not configured: the transition happened but no event is recorded.
		slog.Debug("alerter: alert skipped (webhook not configured or alerts disabled)", "kind", kind, "endpoint_id", endpointID)
		return
	}

	sentOK := true
	if err := e.sender.Send(ctx, webhook, message); err != nil {
		slog.Error("alerter: send alert", "kind", kind, "endpoint_id", endpointID, "error", err)
		sentOK = false
	} else {
		slog.Info("alerter: alert sent", "kind", kind, "endpoint_id", endpointID)
	}

	if _, err := e.db.CreateAlertEvent(store.AlertEvent{
		EndpointID: &endpointID,
		Kind:       kind,
		Message:    message.Text,
		SentOK:     sentOK,
	}); err != nil {
		slog.Error("alerter: record alert event", "kind", kind, "endpoint_id", endpointID, "error", err)
	}
}

// buildMessage composes the alert with the model ID, protocol, and the
// latest error summary for down alerts — once, as a Message whose plain text
// is persisted and whose fields render the card (ticket 101: the two
// renderings share this single source).
func (e *Evaluator) buildMessage(endpointID int64, kind string) (Message, error) {
	endpoint, err := e.db.GetEndpoint(endpointID)
	if err != nil {
		return Message{}, fmt.Errorf("load endpoint: %w", err)
	}
	model, err := e.db.GetModel(endpoint.ModelID)
	if err != nil {
		return Message{}, fmt.Errorf("load model: %w", err)
	}

	if kind == store.AlertKindRecovered {
		return Message{
			Text: fmt.Sprintf("【HubScope】端点恢复:模型 %s(%s)已恢复正常。",
				model.ModelID, endpoint.Protocol),
			Title:    "端点恢复",
			Template: templateGreen,
			Fields: []Field{
				{Label: "模型", Value: model.ModelID},
				{Label: "协议", Value: endpoint.Protocol},
			},
		}, nil
	}

	lastError := "未知错误"
	if latest, err := e.db.LatestProbe(endpointID); err == nil &&
		latest != nil && !latest.OK && latest.ErrorSummary != nil {
		lastError = *latest.ErrorSummary
	}
	return Message{
		Text: fmt.Sprintf("【HubScope】端点告警:模型 %s(%s)已连续 %d 次探测失败,最近错误:%s",
			model.ModelID, endpoint.Protocol, status.DownThreshold, lastError),
		Title:    "端点告警",
		Template: templateRed,
		Fields: []Field{
			{Label: "模型", Value: model.ModelID},
			{Label: "协议", Value: endpoint.Protocol},
			{Label: "连续失败", Value: fmt.Sprintf("%d 次", status.DownThreshold)},
			{Label: "最近错误", Value: lastError},
		},
	}, nil
}
