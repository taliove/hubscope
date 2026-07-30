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
// may be edited or deleted before the flush runs). family is the vendor
// group dimension (spec 0017 ticket 3).
type endpointAlert struct {
	hubName   string
	modelID   string
	protocol  string
	family    string
	lastError string // down transitions only
	text      string // per-endpoint plain text persisted on the endpoint event
}

// bufferedTransition is one endpoint up/down transition waiting for the
// window flush. Its event is already persisted (sent_ok=false, "delivery
// unconfirmed") at decision time — the W5 lazy-rebuild semantics never
// depend on the flush happening. absorbed marks transitions whose vendor
// group was open at decision time (or opened/closed with this very
// transition): they never render into endpoint cards — their story is told
// by the group cards — and their events honestly stay sent_ok=false.
type bufferedTransition struct {
	eventID  int64
	kind     string // store.AlertKindDown or store.AlertKindRecovered
	alert    endpointAlert
	absorbed bool
}

// bufferLocked adds one transition to the window, arming the flush timer on
// the first pending item. Also keeps the quiet-boundary timer in sync — a
// decision point is where a settings change gets picked up. Called with
// e.mu held.
func (e *Evaluator) bufferLocked(t bufferedTransition) {
	e.pending = append(e.pending, t)
	e.armWindowLocked()
	e.syncQuietTimerLocked()
}

// bufferGroupLocked adds one group transition to the same window. Called
// with e.mu held.
func (e *Evaluator) bufferGroupLocked(g bufferedGroupTransition) {
	e.groupPending = append(e.groupPending, g)
	e.armWindowLocked()
	e.syncQuietTimerLocked()
}

// armWindowLocked arms the flush timer on the first pending item. The
// window starts at the first buffered transition (fixed, not sliding):
// anything arriving while the timer is armed joins the same flush. Called
// with e.mu held.
func (e *Evaluator) armWindowLocked() {
	if e.windowTimer != nil {
		return
	}
	e.windowFiresAt = e.clock.Now().Add(alertWindow)
	timer := e.clock.NewTimer(alertWindow)
	e.windowTimer = timer
	go func() {
		<-timer.C()
		e.mu.Lock()
		defer e.mu.Unlock()
		e.flushLocked(context.Background())
	}()
}

// flushLocked sends every buffered transition: group cards first (the
// vendor-level fault is the headline; spec 0017 ticket 3), then one
// aggregated endpoint card per kind (down red, recovered green — bad news
// and good news never share a card) for the transitions not absorbed into a
// group. Every sent card writes its events' sent_ok back and records one
// hub-less batch event carrying the actual text sent and the real delivery
// result. Sends happen under e.mu like every alerter send (W5); failures
// are not retried. Called with e.mu held.
//
// Quiet gate (spec 0017 ticket 4, GH #67): the flush holds when the
// window's scheduled fire time OR its start falls inside quiet hours — a
// window born inside quiet yields entirely to the end-of-window summary
// (GH #67 MEDIUM-1), so a transition at 06:59:30 never produces both a
// 07:00 summary entry and a 07:00:30 card. Both checks stay instant
// judgments on the window's own two endpoints, not an interval filter over
// "what happened inside the period" (the GH #66 HIGH-1 discipline). The
// buffered transitions' events stay sent_ok=false ("delivery unconfirmed")
// until the summary confirms the ones still open; no batch event is
// recorded for a held flush. The state machine and the event log are
// unaffected (transitions keep landing at decision time). The scheduled
// fire time — not the handler's run time — is evaluated, so a flush
// executed late (fake-clock jumps in tests) still honors the quiet state
// of the moment the window was due; under the real clock the two are
// milliseconds apart.
func (e *Evaluator) flushLocked(ctx context.Context) {
	pending := e.pending
	groupPending := e.groupPending
	e.pending = nil
	e.groupPending = nil
	e.windowTimer = nil
	firesAt := e.windowFiresAt
	e.windowFiresAt = time.Time{}
	e.syncQuietTimerLocked()
	if len(pending) == 0 && len(groupPending) == 0 {
		return
	}
	if firesAt.IsZero() {
		firesAt = e.clock.Now()
	}

	q, err := e.quietHoursLocked()
	if err != nil {
		slog.Error("alerter: read quiet-hours settings for window flush", "error", err)
		return
	}
	if q.active() && (q.contains(firesAt) || q.contains(firesAt.Add(-alertWindow))) {
		slog.Debug("alerter: window flush held by quiet hours",
			"transitions", len(pending), "group_transitions", len(groupPending))
		return
	}

	// A quiet summary that ran between decision and flush confirmed the
	// anchors it reported (sent_ok=true): drop transitions confirmed since,
	// so the flush never re-tells a story the summary already told. This
	// closes the decision-exactly-at-07:00:00 race the start-side gate
	// cannot see (window start on the loud side, yet the summary already
	// covered the endpoint).
	confirmed, err := e.confirmedTransitionsLocked(pending, groupPending)
	if err != nil {
		slog.Error("alerter: check confirmed events for window flush", "error", err)
		return
	}
	if len(confirmed) > 0 {
		pending = dropConfirmed(pending, confirmed,
			func(t bufferedTransition) int64 { return t.eventID },
			func(t bufferedTransition) (string, string) { return t.kind, t.alert.modelID })
		groupPending = dropConfirmed(groupPending, confirmed,
			func(g bufferedGroupTransition) int64 { return g.eventID },
			func(g bufferedGroupTransition) (string, string) { return g.kind, g.family })
		if len(pending) == 0 && len(groupPending) == 0 {
			slog.Debug("alerter: window flush skipped, all transitions already confirmed by a quiet summary")
			return
		}
	}

	webhook, err := e.db.GetSetting(store.SettingLarkWebhookURL, "")
	if err != nil {
		slog.Error("alerter: read webhook setting for window flush", "error", err)
		return
	}
	if webhook == "" {
		// Unconfigured between transition and flush: nothing is sent and
		// the events honestly stay sent_ok=false — "delivery unconfirmed".
		slog.Debug("alerter: window flush skipped (webhook not configured)",
			"transitions", len(pending), "group_transitions", len(groupPending))
		return
	}

	// Endpoint down transitions buffered before their group opened inside
	// this same window carry no story beyond what the frozen group card
	// already lists (its faulty snapshot names every member alerted at the
	// trigger), so they are absorbed — by member identity, not by family.
	// Anything the group cards do not mention still renders as an endpoint
	// card: recoveries (a group card never tells an endpoint recovery that
	// happened after its snapshot froze), and endpoints that healed before
	// the trigger (absent from the faulty snapshot). Family-wide absorption
	// would swallow both (check GH #66 HIGH-1).
	//
	// The two-layer mechanism (decision-time absorbed flag + this flush
	// fallback) thus settles on: fallback = story coverage set + ordered
	// consumption (check GH #66 MEDIUM-1). The set is REPLAYED over
	// groupPending in decision order — a group_down adds its frozen faulty
	// members, a group_recovered removes its frozen 已恢复 members — never
	// plainly unioned: a union misses that a later card revoked coverage
	// (the green card told the member's recovery, closing its fault story),
	// and would swallow that member's re-down inside the same window. The
	// replay's known over-report: an early down of a member whose story the
	// green card closed renders as an endpoint card duplicating the red
	// card's snapshot — the safe direction.
	covered := map[string]map[groupMemberRef]bool{}
	for _, g := range groupPending {
		set, ok := covered[g.family]
		if !ok {
			set = map[groupMemberRef]bool{}
			covered[g.family] = set
		}
		switch g.kind {
		case store.AlertKindGroupDown:
			for _, m := range g.faulty {
				set[m] = true
			}
		case store.AlertKindGroupRecovered:
			for _, m := range g.recovered {
				delete(set, m)
			}
		}
	}

	for _, g := range groupPending {
		e.sendCardLocked(ctx, webhook, buildGroupMessage(g), []int64{g.eventID})
	}

	var renderable []bufferedTransition
	for _, t := range pending {
		if t.absorbed {
			continue
		}
		if t.kind == store.AlertKindDown {
			member := groupMemberRef{hubName: t.alert.hubName, modelID: t.alert.modelID, protocol: t.alert.protocol}
			if set, ok := covered[t.alert.family]; ok && set[member] {
				continue
			}
		}
		renderable = append(renderable, t)
	}

	for _, kind := range []string{store.AlertKindDown, store.AlertKindRecovered} {
		var group []bufferedTransition
		for _, t := range renderable {
			if t.kind == kind {
				group = append(group, t)
			}
		}
		if len(group) == 0 {
			continue
		}
		ids := make([]int64, 0, len(group))
		for _, t := range group {
			ids = append(ids, t.eventID)
		}
		e.sendCardLocked(ctx, webhook, buildBatchMessage(kind, group), ids)
	}
}

// confirmedTransitionsLocked returns which of the buffered transitions'
// events have been confirmed (sent_ok=true) since decision time — today the
// only writer of a confirmation outside a flush is the quiet-hours summary.
// Called with e.mu held.
func (e *Evaluator) confirmedTransitionsLocked(pending []bufferedTransition, groupPending []bufferedGroupTransition) (map[int64]bool, error) {
	ids := make([]int64, 0, len(pending)+len(groupPending))
	for _, t := range pending {
		ids = append(ids, t.eventID)
	}
	for _, g := range groupPending {
		ids = append(ids, g.eventID)
	}
	return e.db.ConfirmedAlertEvents(ids)
}

// dropConfirmed removes buffered transitions whose event a quiet summary
// already confirmed between decision and flush (endpoint and group
// transitions share the one implementation). The in-place filter
// (pending[:0]) preserves the callers' backing-array reuse. describe
// returns the transition kind and its log subject (model ID / family name).
func dropConfirmed[T any](pending []T, confirmed map[int64]bool, eventIDOf func(T) int64, describe func(T) (kind, subject string)) []T {
	out := pending[:0]
	for _, t := range pending {
		if confirmed[eventIDOf(t)] {
			kind, subject := describe(t)
			slog.Debug("alerter: transition dropped from flush, already confirmed by a quiet summary",
				"kind", kind, "subject", subject)
			continue
		}
		out = append(out, t)
	}
	return out
}

// sendCardLocked delivers one card to the webhook, writes the delivery
// result back to the given events, and records one hub-less batch event
// carrying the actual text sent and the real delivery result (spec 0017
// story 31: the history shows what readers saw). A failed send is not
// retried and the events stay sent_ok=false — honest, and auditable.
// Called with e.mu held.
func (e *Evaluator) sendCardLocked(ctx context.Context, webhook string, message Message, eventIDs []int64) {
	sentOK := true
	if err := e.sender.Send(ctx, webhook, message); err != nil {
		slog.Error("alerter: send aggregated alert", "events", len(eventIDs), "error", err)
		sentOK = false
	} else {
		slog.Info("alerter: aggregated alert sent", "events", len(eventIDs))
	}
	if err := e.db.UpdateAlertEventsSentOK(eventIDs, sentOK); err != nil {
		slog.Error("alerter: write back alert delivery results", "error", err)
	}
	if _, err := e.db.CreateAlertEvent(store.AlertEvent{
		Kind:    store.AlertKindBatch,
		Message: message.Text,
		SentOK:  sentOK,
	}); err != nil {
		slog.Error("alerter: record batch alert event", "error", err)
	}
}

// hubBucket is one hub's share of an aggregated card, preserving first-seen
// item order within the bucket.
type hubBucket[T any] struct {
	name  string
	items []T
}

// bucketByHub groups items by hub name, preserving first-seen hub and item
// order (deterministic rendering for readers and tests) — the one
// implementation behind the endpoint card's per-hub sections and the group
// card's per-hub member sections. hubOf extracts the hub name; valueOf
// projects the item into what the card renders.
func bucketByHub[In, Out any](items []In, hubOf func(In) string, valueOf func(In) Out) []hubBucket[Out] {
	var buckets []hubBucket[Out]
	index := map[string]int{}
	for _, item := range items {
		hub := hubOf(item)
		i, ok := index[hub]
		if !ok {
			i = len(buckets)
			index[hub] = i
			buckets = append(buckets, hubBucket[Out]{name: hub})
		}
		buckets[i].items = append(buckets[i].items, valueOf(item))
	}
	return buckets
}

// buildBatchMessage composes the aggregated card for one kind: per-hub
// sections in the detail block, plus the plain-text mirror persisted on the
// batch event. One shape serves any group size — the single-endpoint alert
// is the N=1 case of the same aggregation.
func buildBatchMessage(kind string, group []bufferedTransition) Message {
	sections := bucketByHub(group,
		func(t bufferedTransition) string { return t.alert.hubName },
		func(t bufferedTransition) endpointAlert { return t.alert })

	hubWord := fmt.Sprintf("%d 个", len(sections))
	var detailSections, textSections []string
	for _, sec := range sections {
		header := sec.name
		if kind == store.AlertKindDown && len(sec.items) >= 2 {
			header += hubFaultAnnotation
		}
		var lines, names []string
		for _, a := range sec.items {
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
