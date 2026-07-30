package alerter

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/taliove/hubscope/internal/classifier"
	"github.com/taliove/hubscope/internal/store"
)

// groupTriggerMinCount is the absolute floor of the group alert trigger
// (spec 0017 — frozen constant, explicitly not a config surface): a vendor
// group opens only when at least this many of its enabled endpoints are
// alerted, so single-endpoint families never trigger (their alert is the
// endpoint alert's job) and one failure in a two-endpoint family stays an
// endpoint story. The proportional half (≥50%) is computed with integer
// math at the evaluation site.
const groupTriggerMinCount = 2

// bufferedGroupTransition is one vendor group alert opening/closing waiting
// for the window flush. Its event is already persisted (sent_ok=false,
// "delivery unconfirmed") at decision time, and the card content is frozen
// here at the same moment — the flush renders this snapshot even if group
// membership or endpoint states shift inside the window (the same freeze
// discipline as endpointAlert).
type bufferedGroupTransition struct {
	eventID   int64
	kind      string // store.AlertKindGroupDown or store.AlertKindGroupRecovered
	family    string
	total     int              // enabled endpoints of the family at decision time
	faulty    []groupMemberRef // alerted members (down detail / 仍故障 section)
	recovered []groupMemberRef // 已恢复 section (group_recovered only)
}

// groupMemberRef is one group member's frozen rendering identity.
type groupMemberRef struct {
	hubName  string
	modelID  string
	protocol string
}

// isGroupAlerted reports whether the vendor group currently has an open
// group alert, lazily rebuilding the in-memory flag from persisted group
// events on first sight — the group counterpart of isAlerted, deliberately
// fed by its own Latest query so the two state machines never pollute each
// other. The evaluator is the only writer of group events, so caching a
// negative answer is safe.
func (e *Evaluator) isGroupAlerted(family string) (bool, error) {
	if flagged, ok := e.groupAlerted[family]; ok {
		return flagged, nil
	}
	latest, err := e.db.LatestGroupEvent(family)
	if err != nil {
		return false, err
	}
	flagged := latest != nil && latest.Kind == store.AlertKindGroupDown
	e.groupAlerted[family] = flagged
	return flagged, nil
}

// evaluateGroupLocked recomputes one vendor family's group alert state after
// an endpoint transition (spec 0017 ticket 3, GH #66). It runs at the
// transition's decision point — the same cut as the endpoint event — so the
// share below is computed from the just-flipped alerted flags; the
// denominator is the family's enabled-endpoint set at evaluation time.
// Opens the group at ≥50% and ≥groupTriggerMinCount alerted, closes it once
// the share falls strictly below 50%. Called with e.mu held.
func (e *Evaluator) evaluateGroupLocked(family string) {
	// The "other" bucket is not a vendor: unclassified models share nothing
	// but the fallback label, so a majority outage among them is a hub-side
	// story (already covered by the window's suspected-hub-fault
	// annotation), not a vendor group fault.
	if family == "" || family == classifier.DefaultFamily {
		return
	}

	members, err := e.db.ListEnabledEndpointsByFamily(family)
	if err != nil {
		slog.Error("alerter: list family endpoints for group evaluation", "family", family, "error", err)
		return
	}
	if len(members) == 0 {
		// No enabled members: nothing can trigger, and an open group whose
		// members were all disabled stays open until a transition re-enables
		// evaluation (documented edge — disabling is not a transition).
		return
	}

	open, err := e.isGroupAlerted(family)
	if err != nil {
		slog.Error("alerter: rebuild group alert state", "family", family, "error", err)
		return
	}

	faulty := []store.FamilyEndpoint{}
	for _, m := range members {
		flagged, err := e.isAlerted(m.EndpointID)
		if err != nil {
			slog.Error("alerter: rebuild endpoint alert state for group evaluation",
				"family", family, "endpoint_id", m.EndpointID, "error", err)
			return
		}
		if flagged {
			faulty = append(faulty, m)
		}
	}

	switch {
	case !open && len(faulty) >= groupTriggerMinCount && 2*len(faulty) >= len(members):
		e.groupTransitionLocked(family, store.AlertKindGroupDown, members, faulty)
	case open && 2*len(faulty) < len(members):
		e.groupTransitionLocked(family, store.AlertKindGroupRecovered, members, faulty)
	}
}

// groupTransitionLocked flips the in-memory group flag, records the group
// event at decision time, and buffers the frozen card content into the
// aggregation window — mirroring transition(): the flag flips even when the
// webhook is unconfigured (the group state change counts as reported, W5),
// while an unconfigured/disabled setup records no event at all. Registered
// edge (check GH #66 LOW-1, behavior deliberately unchanged): a flag
// flipped while unconfigured lives only in memory, so after the webhook is
// configured the first card a reader sees can be a group_recovered with no
// preceding group_down, and a restart rebuilds from events only (no open
// state) — the exact trade the per-endpoint alerted flag already makes.
// Called with e.mu held.
func (e *Evaluator) groupTransitionLocked(family, kind string, members, faulty []store.FamilyEndpoint) {
	e.groupAlerted[family] = kind == store.AlertKindGroupDown

	webhook, err := e.db.GetSetting(store.SettingLarkWebhookURL, "")
	if err != nil {
		slog.Error("alerter: read webhook setting for group transition", "error", err)
		return
	}
	enabled, err := e.db.GetSettingBool(store.SettingAlertEnabled, store.DefaultAlertEnabled)
	if err != nil {
		slog.Error("alerter: read alert_enabled setting for group transition", "error", err)
		return
	}
	if webhook == "" || !enabled {
		slog.Debug("alerter: group alert skipped (webhook not configured or alerts disabled)", "kind", kind, "family", family)
		return
	}

	g := bufferedGroupTransition{
		kind:   kind,
		family: family,
		total:  len(members),
	}
	for _, m := range faulty {
		g.faulty = append(g.faulty, groupMemberRef{hubName: m.HubName, modelID: m.ModelID, protocol: m.Protocol})
	}
	if kind == store.AlertKindGroupRecovered {
		for _, m := range e.recoveredGroupMembersLocked(family, members, faulty) {
			g.recovered = append(g.recovered, groupMemberRef{hubName: m.HubName, modelID: m.ModelID, protocol: m.Protocol})
		}
	}

	message := buildGroupMessage(g)
	event, err := e.db.CreateAlertEvent(store.AlertEvent{
		GroupKey: &family,
		Kind:     kind,
		Message:  message.Text,
		SentOK:   false, // delivery unconfirmed until the window flush
	})
	if err != nil {
		slog.Error("alerter: record group alert event", "kind", kind, "family", family, "error", err)
		return
	}
	g.eventID = event.ID
	e.bufferGroupLocked(g)
}

// recoveredGroupMembersLocked computes the 已恢复 section of a group
// recovery card: enabled members not currently faulty whose latest
// down/recovered event is a recovery recorded after the group opened. Event
// IDs are AUTOINCREMENT-monotonic, so the id comparison is the precise
// "recovered during this open" test (created_at has only second precision).
// Members with no events, or whose outage predates the group and is still
// open, are correctly excluded (the latter are in faulty).
func (e *Evaluator) recoveredGroupMembersLocked(family string, members, faulty []store.FamilyEndpoint) []store.FamilyEndpoint {
	opened, err := e.db.LatestGroupEvent(family)
	if err != nil {
		slog.Error("alerter: load group open event for recovery detail", "family", family, "error", err)
		return nil
	}
	if opened == nil || opened.Kind != store.AlertKindGroupDown {
		return nil
	}

	faultyIDs := make(map[int64]bool, len(faulty))
	for _, m := range faulty {
		faultyIDs[m.EndpointID] = true
	}
	recovered := []store.FamilyEndpoint{}
	for _, m := range members {
		if faultyIDs[m.EndpointID] {
			continue
		}
		latest, err := e.db.LatestDownRecoveryEvent(m.EndpointID)
		if err != nil {
			slog.Error("alerter: load endpoint recovery event for group recovery detail",
				"family", family, "endpoint_id", m.EndpointID, "error", err)
			continue
		}
		if latest != nil && latest.Kind == store.AlertKindRecovered && latest.ID > opened.ID {
			recovered = append(recovered, m)
		}
	}
	return recovered
}

// buildGroupMessage composes the group card from the frozen snapshot: a red
// vendor-titled card with per-hub faulty-member detail for group_down, or a
// green card with 已恢复 / 仍故障 sections for group_recovered. Text is the
// plain-text mirror persisted on the group event (and later on the batch
// event), Title/Template/Fields/Detail the interactive card.
func buildGroupMessage(g bufferedGroupTransition) Message {
	if g.kind == store.AlertKindGroupRecovered {
		recoveredLines := recoverySectionLines(g.recovered)
		faultyLines := recoverySectionLines(g.faulty)
		return Message{
			Text: fmt.Sprintf("【HubScope】厂商组恢复:厂商 %s 组告警解除,已恢复 %d 个(%s),仍故障 %d 个(%s)。",
				g.family, len(g.recovered), memberNames(g.recovered), len(g.faulty), memberNames(g.faulty)),
			Title:    "厂商组恢复:" + g.family,
			Template: templateGreen,
			Fields: []Field{
				{Label: "厂商", Value: g.family},
				{Label: "已恢复", Value: fmt.Sprintf("%d 个", len(g.recovered))},
				{Label: "仍故障", Value: fmt.Sprintf("%d 个", len(g.faulty))},
			},
			Detail: "**已恢复**\n" + strings.Join(recoveredLines, "\n") +
				"\n\n**仍故障**\n" + strings.Join(faultyLines, "\n"),
		}
	}

	sections := bucketByHub(g.faulty,
		func(m groupMemberRef) string { return m.hubName },
		func(m groupMemberRef) groupMemberRef { return m })
	var detailSections, textSections []string
	for _, sec := range sections {
		var lines, names []string
		for _, m := range sec.items {
			names = append(names, fmt.Sprintf("%s(%s)", m.modelID, m.protocol))
			lines = append(lines, fmt.Sprintf("· %s(%s)", m.modelID, m.protocol))
		}
		detailSections = append(detailSections, "**"+sec.name+"**\n"+strings.Join(lines, "\n"))
		textSections = append(textSections, sec.name+":"+strings.Join(names, "、"))
	}
	return Message{
		Text: fmt.Sprintf("【HubScope】厂商组告警:厂商 %s 组内 %d/%d 个端点故障:%s",
			g.family, len(g.faulty), g.total, strings.Join(textSections, ";")),
		Title:    "厂商组告警:" + g.family,
		Template: templateRed,
		Fields: []Field{
			{Label: "厂商", Value: g.family},
			{Label: "故障端点", Value: fmt.Sprintf("%d/%d 个", len(g.faulty), g.total)},
		},
		Detail: strings.Join(detailSections, "\n\n"),
	}
}

// recoverySectionLines renders one 已恢复/仍故障 section: one line per member
// with its hub inline, or an explicit 无 so an empty section never reads as
// a rendering accident.
func recoverySectionLines(members []groupMemberRef) []string {
	if len(members) == 0 {
		return []string{"· 无"}
	}
	lines := make([]string, 0, len(members))
	for _, m := range members {
		lines = append(lines, fmt.Sprintf("· %s(%s) · %s", m.modelID, m.protocol, m.hubName))
	}
	return lines
}

// memberNames renders a compact name list for the plain-text mirror.
func memberNames(members []groupMemberRef) string {
	if len(members) == 0 {
		return "无"
	}
	names := make([]string, 0, len(members))
	for _, m := range members {
		names = append(names, m.modelID)
	}
	return strings.Join(names, "、")
}
