// Recent-events card derivations (GH #132, UI v2 O5; GH #138 reference
// card): the dashboard's 近期事件 section renders at most four cards from
// the alert feed. Every card field — title, impact scope, event chip
// (double-track: color = kind tag type, word = incident state or kind
// word), duration wording — is derived here so the component only renders
// (alertTimeline.ts / format.ts centralization precedent).
import type { TagProps } from 'element-plus'
import type { AlertEvent } from '@/api/settings'
import type { OverviewEntry } from '@/api/types'
import { alertKindLabel, alertKindTagType } from './alertKind'
import { formatDuration } from './format'
import type { IncidentDuration } from './alertTimeline'

// The section fetches a wider window than it shows so FIFO incident pairing
// can see a recovery that sits beyond the fourth card; only after pairing
// are the newest four taken. Known caliber (AlertsView's documented one):
// a down whose recovery falls outside the fetched window reads 进行中.
export const RECENT_EVENTS_FETCH_LIMIT = 20
export const RECENT_EVENTS_CARD_LIMIT = 4

// selectRecentEvents takes the newest N events; the API returns newest
// first, so this is a plain slice. Pairing must be computed by the caller
// on the full fetched window BEFORE this selection, so the card subset can
// never change a duration.
export function selectRecentEvents(
  events: AlertEvent[],
  limit = RECENT_EVENTS_CARD_LIMIT,
): AlertEvent[] {
  return events.slice(0, limit)
}

// eventTitle: the first line of the alert message — aggregate messages
// carry per-endpoint detail on later lines (window.go / group.go message
// builders). The component clamps the render to two lines and keeps the
// full message on the title attribute.
export function eventTitle(message: string): string {
  return message.split('\n')[0].trim()
}

// buildEndpointModelMap resolves endpoint_id → model_id from the overview
// payload the dashboard already holds — the section issues no second
// overview request.
export function buildEndpointModelMap(entries: OverviewEntry[]): Map<number, string> {
  const map = new Map<number, string>()
  for (const entry of entries) {
    map.set(entry.endpoint_id, entry.model_id)
  }
  return map
}

// impactText: the 影响范围 line. Endpoint events name the affected model
// (a deleted endpoint drops out of the overview and falls back to its raw
// id — the AlertsView precedent, the card must still render); vendor-group
// events name the group; hub-less events (test / batch / quiet summary /
// score comparisons) affect no model and show the event-kind word instead.
export function impactText(
  ev: AlertEvent,
  endpointModels: ReadonlyMap<number, string>,
): string {
  if (ev.group_key !== null) return `影响 ${ev.group_key}`
  if (ev.endpoint_id !== null) {
    return `影响 ${endpointModels.get(ev.endpoint_id) ?? `#${ev.endpoint_id}`}`
  }
  return alertKindLabel(ev.kind)
}

// Event chip, double-track caliber (GH #138, reference-design replication —
// main's dispatch ruling): COLOR = the event kind's tag type
// (alertKindTagType, the same mapping the /alerts timeline consumes — all
// eleven kinds covered, never a hand-rolled mapping); WORD = the incident
// state for opener events (down / group_down with a pairing entry: 进行中
// while unclosed, 已恢复 once the recovery arrived — the GH #132 pairing
// caliber unchanged) or the event-kind word for point-in-time events
// (recovered / test / batch / score_drop / retire_pending / … →
// alertKindLabel). Every card renders a chip — the reference design shows
// one per card, and point-in-time events carry their kind word instead of
// rendering bare. Registered deviation from the reference mock: its
// multi-colored 已恢复 chips do not map onto our data model — a recovered
// INCIDENT reads 已恢复 on the opener's danger tone (the event IS a down),
// while the recovered EVENT itself reads 恢复 on success.
export type EventChip = { text: string; tone: TagProps['type'] }

export function eventChip(
  ev: AlertEvent,
  durations: ReadonlyMap<number, IncidentDuration>,
): EventChip {
  const d = durations.get(ev.id)
  // pairIncidentDurations keys exactly the opener ids, so a missing entry
  // also covers scopeless openers (a down with a null endpoint_id) — those
  // fall through to the kind word like any point-in-time event.
  const text = d ? (d.state === 'ongoing' ? '进行中' : '已恢复') : alertKindLabel(ev.kind)
  return { text, tone: alertKindTagType(ev.kind) }
}

// incidentDurationText: paired incidents read 持续 X; unpaired openers read
// 已持续 X measured against `now` (injected so tests stay deterministic);
// non-incident events have no duration cell at all. The span format reuses
// formatDuration — the same wording the /alerts timeline renders.
export function incidentDurationText(
  ev: AlertEvent,
  durations: ReadonlyMap<number, IncidentDuration>,
  now: Date,
): string {
  const d = durations.get(ev.id)
  if (!d) return ''
  if (d.state === 'paired') return `持续 ${formatDuration(d.ms)}`
  const start = new Date(ev.created_at).getTime()
  const elapsed = Number.isNaN(start) ? 0 : Math.max(0, now.getTime() - start)
  return `已持续 ${formatDuration(elapsed)}`
}
