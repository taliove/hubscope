package alerter

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/taliove/hubscope/internal/status"
	"github.com/taliove/hubscope/internal/store"
)

// alertWindow is the aggregation buffer length (spec 0017, ADR 0014 —
// frozen constant, explicitly not a config surface): transitions inside one
// window merge into a single card per kind; a single-endpoint alert pays at
// most this much delay in exchange.
const alertWindow = 60 * time.Second

// hubFaultAnnotation flags a hub section holding ≥2 endpoints inside one
// aggregated down card: several endpoints of one hub failing together
// points at the hub, not at the models. Recovery cards stay unannotated —
// the suspicion wording belongs to the fault, not to the all-clear.
const hubFaultAnnotation = "(疑似 Hub 侧故障)"

// endpointAlert captures everything the window flush needs to render one
// endpoint inside an aggregated card, frozen at decision time (the endpoint
// may be edited or deleted before the flush runs).
type endpointAlert struct {
	hubName   string
	modelID   string
	protocol  string
	lastError string // down transitions only
	text      string // per-endpoint plain text persisted on the endpoint event
}

// bufferedTransition is one endpoint up/down transition waiting for the
// window flush. Its event is already persisted (sent_ok=false, "delivery
// unconfirmed") at decision time — the W5 lazy-rebuild semantics never
// depend on the flush happening.
type bufferedTransition struct {
	eventID int64
	kind    string // store.AlertKindDown or store.AlertKindRecovered
	alert   endpointAlert
}

// bufferLocked adds one transition to the window, arming the flush timer on
// the first pending item. The window starts at the first buffered
// transition (fixed, not sliding): anything arriving while the timer is
// armed joins the same flush. Called with e.mu held.
func (e *Evaluator) bufferLocked(t bufferedTransition) {
	e.pending = append(e.pending, t)
	if e.windowTimer != nil {
		return
	}
	timer := e.clock.NewTimer(alertWindow)
	e.windowTimer = timer
	go func() {
		<-timer.C()
		e.mu.Lock()
		defer e.mu.Unlock()
		e.flushLocked(context.Background())
	}()
}

// flushLocked sends every buffered transition as one aggregated card per
// kind (down red, recovered green — bad news and good news never share a
// card), writes each event's sent_ok back, and records one hub-less batch
// event per card carrying the actual text sent and the real delivery
// result. Sends happen under e.mu like every alerter send (W5); failures
// are not retried. Called with e.mu held.
func (e *Evaluator) flushLocked(ctx context.Context) {
	pending := e.pending
	e.pending = nil
	e.windowTimer = nil
	if len(pending) == 0 {
		return
	}

	webhook, err := e.db.GetSetting(store.SettingLarkWebhookURL, "")
	if err != nil {
		slog.Error("alerter: read webhook setting for window flush", "error", err)
		return
	}
	if webhook == "" {
		// Unconfigured between transition and flush: nothing is sent and
		// the events honestly stay sent_ok=false — "delivery unconfirmed".
		slog.Debug("alerter: window flush skipped (webhook not configured)", "transitions", len(pending))
		return
	}

	for _, kind := range []string{store.AlertKindDown, store.AlertKindRecovered} {
		var group []bufferedTransition
		for _, t := range pending {
			if t.kind == kind {
				group = append(group, t)
			}
		}
		if len(group) == 0 {
			continue
		}

		message := buildBatchMessage(kind, group)
		sentOK := true
		if err := e.sender.Send(ctx, webhook, message); err != nil {
			slog.Error("alerter: send aggregated alert", "kind", kind, "transitions", len(group), "error", err)
			sentOK = false
		} else {
			slog.Info("alerter: aggregated alert sent", "kind", kind, "transitions", len(group))
		}

		ids := make([]int64, 0, len(group))
		for _, t := range group {
			ids = append(ids, t.eventID)
		}
		if err := e.db.UpdateAlertEventsSentOK(ids, sentOK); err != nil {
			slog.Error("alerter: write back alert delivery results", "kind", kind, "error", err)
		}
		if _, err := e.db.CreateAlertEvent(store.AlertEvent{
			Kind:    store.AlertKindBatch,
			Message: message.Text,
			SentOK:  sentOK,
		}); err != nil {
			slog.Error("alerter: record batch alert event", "kind", kind, "error", err)
		}
	}
}

// hubSection is one hub's share of an aggregated card, preserving
// first-seen endpoint order.
type hubSection struct {
	name   string
	alerts []endpointAlert
}

// groupByHub buckets transitions by hub name, preserving first-seen hub and
// endpoint order (deterministic rendering for readers and tests).
func groupByHub(group []bufferedTransition) []hubSection {
	var sections []hubSection
	index := map[string]int{}
	for _, t := range group {
		i, ok := index[t.alert.hubName]
		if !ok {
			i = len(sections)
			index[t.alert.hubName] = i
			sections = append(sections, hubSection{name: t.alert.hubName})
		}
		sections[i].alerts = append(sections[i].alerts, t.alert)
	}
	return sections
}

// buildBatchMessage composes the aggregated card for one kind: per-hub
// sections in the detail block, plus the plain-text mirror persisted on the
// batch event. One shape serves any group size — the single-endpoint alert
// is the N=1 case of the same aggregation.
func buildBatchMessage(kind string, group []bufferedTransition) Message {
	sections := groupByHub(group)

	hubWord := fmt.Sprintf("%d 个", len(sections))
	var detailSections, textSections []string
	for _, sec := range sections {
		header := sec.name
		if kind == store.AlertKindDown && len(sec.alerts) >= 2 {
			header += hubFaultAnnotation
		}
		var lines, names []string
		for _, a := range sec.alerts {
			names = append(names, fmt.Sprintf("%s(%s)", a.modelID, a.protocol))
			if kind == store.AlertKindRecovered {
				lines = append(lines, fmt.Sprintf("· %s(%s):已恢复正常", a.modelID, a.protocol))
			} else {
				lines = append(lines, fmt.Sprintf("· %s(%s):连续 %d 次探测失败,最近错误:%s",
					a.modelID, a.protocol, status.DownThreshold, a.lastError))
			}
		}
		detailSections = append(detailSections, "**"+header+"**\n"+strings.Join(lines, "\n"))
		textSections = append(textSections, header+":"+strings.Join(names, "、"))
	}

	if kind == store.AlertKindRecovered {
		return Message{
			Text: fmt.Sprintf("【HubScope】端点恢复:%d 个端点已恢复正常:%s",
				len(group), strings.Join(textSections, ";")),
			Title:    "端点恢复",
			Template: templateGreen,
			Fields: []Field{
				{Label: "恢复端点", Value: fmt.Sprintf("%d 个", len(group))},
				{Label: "涉及 Hub", Value: hubWord},
			},
			Detail: strings.Join(detailSections, "\n\n"),
		}
	}
	return Message{
		Text: fmt.Sprintf("【HubScope】端点告警:%d 个端点连续 %d 次探测失败:%s",
			len(group), status.DownThreshold, strings.Join(textSections, ";")),
		Title:    "端点告警",
		Template: templateRed,
		Fields: []Field{
			{Label: "影响端点", Value: fmt.Sprintf("%d 个", len(group))},
			{Label: "涉及 Hub", Value: hubWord},
		},
		Detail: strings.Join(detailSections, "\n\n"),
	}
}
