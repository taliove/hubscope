package alerter

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/taliove/hubscope/internal/store"
)

// quietHours is the parsed settings triple for the daily quiet window
// (spec 0017 ticket 4, GH #67). Hours are integers 0–23 in the server's
// local timezone; the window may cross midnight (23→7); start == end means
// "not enabled" even when the switch is on.
type quietHours struct {
	enabled bool
	start   int
	end     int
}

// quietScoreDrop is one score_drop alert deferred by the quiet window: its
// event is already persisted (sent_ok=false, "delivery unconfirmed") and the
// frozen message text rides the window-end summary. The pending list is
// in-memory like the window buffer — a restart drops it, and the event
// honestly stays sent_ok=false ("delivery unconfirmed", the same trade as
// the aggregation window's volatile buffer, ADR 0014).
type quietScoreDrop struct {
	eventID int64
	text    string
}

// quietHoursLocked reads the three quiet-hours settings. Called with e.mu
// held (every alerter send path already holds it).
func (e *Evaluator) quietHoursLocked() (quietHours, error) {
	enabled, err := e.db.GetSettingBool(store.SettingQuietHoursEnabled, store.DefaultQuietHoursEnabled)
	if err != nil {
		return quietHours{}, err
	}
	start, err := e.db.GetSettingInt(store.SettingQuietHoursStart, store.DefaultQuietHoursStart)
	if err != nil {
		return quietHours{}, err
	}
	end, err := e.db.GetSettingInt(store.SettingQuietHoursEnd, store.DefaultQuietHoursEnd)
	if err != nil {
		return quietHours{}, err
	}
	return quietHours{enabled: enabled, start: start, end: end}, nil
}

// active reports whether the window can gate sends: the switch is on, the
// bounds differ (start == end means "not enabled"), and both are valid
// hours — a hand-edited out-of-range value disables the window rather than
// warping the boundary math (the settings API validates 0–23 on write).
func (q quietHours) active() bool {
	if !q.enabled || q.start == q.end {
		return false
	}
	return q.start >= 0 && q.start <= 23 && q.end >= 0 && q.end <= 23
}

// contains reports whether now falls inside the window, comparing local
// hours. Cross-midnight windows (start > end) wrap: 23→7 contains 23:30 and
// 06:30 but not 12:00. The boundary instants themselves belong to the loud
// side of the day: 07:00 is no longer quiet, 23:00 already is.
func (q quietHours) contains(now time.Time) bool {
	h := now.Hour()
	if q.start < q.end {
		return h >= q.start && h < q.end
	}
	return h >= q.start || h < q.end
}

// nextBoundary returns the smallest instant strictly after now at which
// contains() flips: the next start:00 or end:00 in now's own location (the
// server-local zone, which is also where the FakeClock's pinned instants
// live — so cross-midnight tests never depend on the runner's TZ).
func (q quietHours) nextBoundary(now time.Time) time.Time {
	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	var best time.Time
	for day := 0; day <= 1; day++ {
		for _, hour := range []int{q.start, q.end} {
			candidate := midnight.AddDate(0, 0, day).Add(time.Duration(hour) * time.Hour)
			if candidate.After(now) && (best.IsZero() || candidate.Before(best)) {
				best = candidate
			}
		}
	}
	return best
}

// syncQuietTimerLocked (re)arms the single quiet-boundary timer so it fires
// at the next window edge. It is called from every alert decision point
// (transition buffering, window flush, campaign handling) and from the
// boundary handler itself, so the boundary chain stays alive as long as
// alerting is active; a settings change is picked up at the next decision
// point (registered edge: with zero alert activity there is also nothing to
// gate or summarize). Disarmed when the window is not active. Called with
// e.mu held.
func (e *Evaluator) syncQuietTimerLocked() {
	q, err := e.quietHoursLocked()
	if err != nil {
		slog.Error("alerter: read quiet-hours settings", "error", err)
		return
	}
	if !q.active() {
		e.disarmQuietTimerLocked()
		return
	}
	now := e.clock.Now()
	next := q.nextBoundary(now)
	if e.quietTimer != nil {
		if !e.quietBoundaryAt.After(now) {
			// The armed boundary has been reached — its fire is buffered on
			// the timer channel but not yet consumed. Leave it to the
			// boundary handler (which re-arms after handling): superseding
			// it here would swap e.quietTimer out from under the in-flight
			// fire and swallow the crossing (race surfaced under -race: a
			// late window flush re-arming past the due boundary). The
			// in-flight goroutine is deliberately NOT released via
			// quietDone: its fire must be consumed, not cancelled.
			return
		}
		if next.Equal(e.quietBoundaryAt) {
			return
		}
		// Supersede: the armed boundary has not been reached, so its timer
		// cannot have fired — stopping it and closing quietDone releases
		// the waiter goroutine deterministically (its channel stays empty).
		//
		// Registered race (check LOW-1, pre-existing exposure, behavior
		// deliberately unchanged): under the REAL clock the boundary can
		// fall due in the microseconds between this guard's now-check and
		// the Stop() below — the fire then races the quietDone close and a
		// due crossing can be cancelled. Worst case the summary is delayed
		// to the next boundary; the deferred events honestly stay
		// sent_ok=false ("delivery unconfirmed") — the safe direction.
		e.disarmQuietTimerLocked()
	}
	e.quietBoundaryAt = next
	timer := e.clock.NewTimer(next.Sub(now))
	done := make(chan struct{})
	e.quietTimer = timer
	e.quietDone = done
	go func() {
		select {
		case <-timer.C():
			e.mu.Lock()
			defer e.mu.Unlock()
			if e.quietTimer != timer {
				// Superseded by a re-arm before the fire was consumed.
				return
			}
			e.quietTimer = nil
			e.quietDone = nil
			e.onQuietBoundaryLocked(context.Background())
		case <-done:
			// Superseded or disarmed before firing: exit instead of
			// parking forever on the stopped timer's channel.
		}
	}()
}

// disarmQuietTimerLocked stops the armed quiet-boundary timer (if any) and
// releases its waiter goroutine by closing quietDone. Callers must not use
// it when a fire is already buffered for consumption (the in-flight guard
// in syncQuietTimerLocked returns before reaching here): there the fire
// branch and the done branch would race, and cancelling a due crossing
// would swallow it. Called with e.mu held.
func (e *Evaluator) disarmQuietTimerLocked() {
	if e.quietTimer == nil {
		return
	}
	e.quietTimer.Stop()
	e.quietTimer = nil
	e.quietBoundaryAt = time.Time{}
	close(e.quietDone)
	e.quietDone = nil
}

// onQuietBoundaryLocked handles one boundary crossing: if the window just
// ended, deliver the summary. The state is evaluated at the current time
// rather than assumed from the armed boundary, so a coarse clock advance
// that skips a whole window (landing inside the next one, or with quiet
// hours since disabled) sends nothing. Re-arms the timer for the next
// boundary either way — one fire produces at most one summary. Called with
// e.mu held.
func (e *Evaluator) onQuietBoundaryLocked(ctx context.Context) {
	defer e.syncQuietTimerLocked()
	q, err := e.quietHoursLocked()
	if err != nil {
		slog.Error("alerter: read quiet-hours settings at boundary", "error", err)
		return
	}
	if !q.active() || q.contains(e.clock.Now()) {
		return // window entered (not ended), disabled, or skipped wholesale
	}
	e.deliverQuietSummaryLocked(ctx, q)
}

// deliverQuietSummaryLocked sends the one quiet-hours end summary. Content
// (spec 0017 stories 22/23/24/29):
//   - still-open endpoints whose anchor event was never delivered
//     (sent_ok=false) — derived from events, never from memory, so a
//     restart cannot corrupt it and an alert delivered before the window is
//     not repeated; endpoints that healed inside the window have a trailing
//     recovery event and drop out;
//   - still-open vendor groups on the same unconfirmed-anchor rule (a
//     reported group folds its member endpoints into the group line).
//
// Registered safe-direction duplicates (check GH #67 HIGH-1, accepted
// behavior; GH #76 will make the summary coverage-set aware and fold
// them): the member fold keys off the group anchor's sent_ok, which cannot
// distinguish "absorbed member whose story a delivered group card already
// told" from "member whose story was never told" — so absorbed members of
// a pre-quiet delivered group, and members of a group whose anchor an
// earlier summary confirmed, reappear here under their individual
// identities. Over-report, never swallow.
//   - the score_drop events deferred inside the window.
//
// An empty summary is never sent and never recorded. The summary confirms
// exactly the anchors it reports (their sent_ok is written back) plus the
// deferred score_drops, and lands as one hub-less quiet_summary event
// carrying the actual text sent — the batch-event precedent. A failed send
// is not retried; the events honestly stay sent_ok=false. Called with e.mu
// held.
func (e *Evaluator) deliverQuietSummaryLocked(ctx context.Context, q quietHours) {
	webhook, err := e.db.GetSetting(store.SettingLarkWebhookURL, "")
	if err != nil {
		slog.Error("alerter: read webhook setting for quiet summary", "error", err)
		return
	}
	if webhook == "" {
		// Unconfigured at summary time: nothing is sent and the deferred
		// events honestly stay sent_ok=false — the window flush's
		// "webhook cleared between transition and flush" precedent.
		slog.Debug("alerter: quiet summary skipped (webhook not configured)",
			"deferred_score_drops", len(e.quietScoreDrops))
		e.quietScoreDrops = nil
		return
	}

	openGroups, err := e.db.ListOpenGroupAlerts()
	if err != nil {
		slog.Error("alerter: list open group alerts for quiet summary", "error", err)
		return
	}
	openEndpoints, err := e.db.ListOpenEndpointAlerts()
	if err != nil {
		slog.Error("alerter: list open endpoint alerts for quiet summary", "error", err)
		return
	}

	// Candidates carry an unconfirmed anchor (sent_ok=false): their story
	// never reached Lark. Confirmed anchors — delivered before the window or
	// by an earlier summary — are not repeated.
	reportedFamilies := map[string]bool{}
	var groups []string
	var confirmIDs []int64
	for _, g := range openGroups {
		if g.SentOK {
			continue
		}
		reportedFamilies[g.GroupKey] = true
		groups = append(groups, g.GroupKey)
		confirmIDs = append(confirmIDs, g.EventID)
	}
	var endpoints []store.OpenEndpointAlert
	for _, ep := range openEndpoints {
		if ep.SentOK {
			continue
		}
		if reportedFamilies[ep.Family] {
			// The group's line in this summary tells the member's story;
			// the member's own event honestly stays sent_ok=false (never
			// individually delivered — the absorption precedent).
			continue
		}
		endpoints = append(endpoints, ep)
		confirmIDs = append(confirmIDs, ep.EventID)
	}
	drops := e.quietScoreDrops

	if len(endpoints) == 0 && len(groups) == 0 && len(drops) == 0 {
		slog.Debug("alerter: quiet window ended with an empty summary, nothing sent")
		return
	}

	message := buildQuietSummaryMessage(q, endpoints, groups, drops)
	sentOK := true
	if err := e.sender.Send(ctx, webhook, message); err != nil {
		slog.Error("alerter: send quiet summary", "error", err)
		sentOK = false
	} else {
		slog.Info("alerter: quiet summary sent",
			"endpoints", len(endpoints), "groups", len(groups), "score_drops", len(drops))
	}
	for _, d := range drops {
		confirmIDs = append(confirmIDs, d.eventID)
	}
	if err := e.db.UpdateAlertEventsSentOK(confirmIDs, sentOK); err != nil {
		slog.Error("alerter: write back quiet summary delivery results", "error", err)
	}
	if _, err := e.db.CreateAlertEvent(store.AlertEvent{
		Kind:    store.AlertKindQuietSummary,
		Message: message.Text,
		SentOK:  sentOK,
	}); err != nil {
		slog.Error("alerter: record quiet summary event", "error", err)
	}
	e.quietScoreDrops = nil // sent or not: no retry, the batch precedent
}

// buildQuietSummaryMessage composes the summary card: a red header (still
// faulty is bad news, spec 0017 Lark card decision), the counts as fields,
// and one section per content class — endpoints grouped by hub, groups by
// family, deferred score_drops by their full frozen text (score_drops
// inside one window are rare; the deferred delivery must carry the
// substance, not a teaser first line — spec axis (a)2). Empty sections
// render an explicit 无 so a missing class never reads as a rendering
// accident. Endpoints and groups are sorted for deterministic rendering.
func buildQuietSummaryMessage(q quietHours, endpoints []store.OpenEndpointAlert, groups []string, drops []quietScoreDrop) Message {
	window := fmt.Sprintf("%02d:00–%02d:00", q.start, q.end)

	sort.Slice(endpoints, func(i, j int) bool {
		if endpoints[i].HubName != endpoints[j].HubName {
			return endpoints[i].HubName < endpoints[j].HubName
		}
		if endpoints[i].ModelID != endpoints[j].ModelID {
			return endpoints[i].ModelID < endpoints[j].ModelID
		}
		return endpoints[i].Protocol < endpoints[j].Protocol
	})
	sort.Strings(groups)

	// Endpoint lines, grouped by hub (first-seen order after the sort).
	var endpointDetail, endpointNames []string
	seenHub := map[string]bool{}
	for _, ep := range endpoints {
		if !seenHub[ep.HubName] {
			seenHub[ep.HubName] = true
			endpointDetail = append(endpointDetail, "**"+ep.HubName+"**")
		}
		endpointDetail = append(endpointDetail, fmt.Sprintf("· %s(%s)", ep.ModelID, ep.Protocol))
		endpointNames = append(endpointNames, fmt.Sprintf("%s:%s(%s)", ep.HubName, ep.ModelID, ep.Protocol))
	}
	if len(endpointDetail) == 0 {
		endpointDetail = []string{"· 无"}
	}

	groupLines := make([]string, 0, len(groups))
	for _, family := range groups {
		groupLines = append(groupLines, "· "+family)
	}
	if len(groupLines) == 0 {
		groupLines = []string{"· 无"}
	}

	dropLines := make([]string, 0, len(drops))
	for _, d := range drops {
		dropLines = append(dropLines, "· "+d.text)
	}
	if len(dropLines) == 0 {
		dropLines = []string{"· 无"}
	}

	text := fmt.Sprintf("【HubScope】静默摘要:静默时段(%s)内告警暂缓发送,现摘要如下——仍故障端点 %d 个:%s;仍故障厂商组 %d 个:%s;静默期内分数大跌 %d 条。",
		window, len(endpoints), strings.Join(endpointNames, "、"), len(groups), strings.Join(groups, "、"), len(drops))
	return Message{
		Text:     text,
		Title:    "静默摘要",
		Template: templateRed,
		Fields: []Field{
			{Label: "静默时段", Value: window},
			{Label: "仍故障端点", Value: fmt.Sprintf("%d 个", len(endpoints))},
			{Label: "仍故障厂商组", Value: fmt.Sprintf("%d 个", len(groups))},
			{Label: "期内分数大跌", Value: fmt.Sprintf("%d 条", len(drops))},
		},
		Detail: "**仍故障端点**\n" + strings.Join(endpointDetail, "\n") +
			"\n\n**仍故障厂商组**\n" + strings.Join(groupLines, "\n") +
			"\n\n**静默期内分数大跌**\n" + strings.Join(dropLines, "\n"),
	}
}
