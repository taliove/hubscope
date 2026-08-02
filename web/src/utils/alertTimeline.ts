// Alert-timeline pure functions (GH #117, spec 0018 §12): time-range
// filtering, down↔recovered duration pairing, and local-date grouping for
// the 故障记录 event timeline. Components render only; every derivation
// lives here so the timeline semantics stay testable (format.ts /
// overviewDots.ts centralization precedent).
import type { AlertEvent, AlertKind } from '@/api/settings'
import { ALERT_KINDS } from '@/utils/alertKind'
import { formatTime, formatDuration } from '@/utils/format'

// Time-range presets offered by the timeline filter bar. 'today' is the
// local calendar day (midnight → now); the others are rolling windows.
export type AlertTimeRange = 'today' | '24h' | '7d' | '30d'

const DAY_MS = 24 * 60 * 60 * 1000

// filterEventsByTimeRange keeps events at or after the range start.
// `now` is injected so the boundary arithmetic stays deterministic in tests.
export function filterEventsByTimeRange(
  events: AlertEvent[],
  range: AlertTimeRange,
  now: Date,
): AlertEvent[] {
  let startMs: number
  if (range === 'today') {
    startMs = new Date(now.getFullYear(), now.getMonth(), now.getDate()).getTime()
  } else {
    const days = range === '24h' ? 1 : range === '7d' ? 7 : 30
    startMs = now.getTime() - days * DAY_MS
  }
  return events.filter((e) => {
    const t = new Date(e.created_at).getTime()
    return !Number.isNaN(t) && t >= startMs && t <= now.getTime()
  })
}

// Duration of a paired incident: recovered_at - down_at in milliseconds.
// 'paired' carries the ms span plus the closing event's id so the inline
// detail (GH #144) can name the recovery WITHOUT re-running the pairing —
// one caliber, one map. 'ongoing' means no matching recovery exists in the
// fetched window (the timeline renders 进行中).
export type IncidentDuration =
  | { state: 'paired'; ms: number; closerId: number }
  | { state: 'ongoing' }

// The incident scope a down/recovered pair shares: endpoint-scoped events
// pair by endpoint_id, vendor-group events (spec 0017) by group_key. Events
// outside the two incident kind pairs have no scope and never pair.
function incidentScopeKey(event: AlertEvent): string | null {
  if (event.kind === 'down' || event.kind === 'recovered') {
    return event.endpoint_id !== null ? `endpoint:${event.endpoint_id}` : null
  }
  if (event.kind === 'group_down' || event.kind === 'group_recovered') {
    return event.group_key !== null ? `group:${event.group_key}` : null
  }
  return null
}

function isOpener(kind: AlertEvent['kind']): boolean {
  return kind === 'down' || kind === 'group_down'
}

// pairIncidentDurations matches every down/group_down event with the first
// later recovered/group_recovered in the same scope, FIFO: when several
// downs arrive without an intervening recovery (duplicate alerts), the
// recovery closes the EARLIEST open down — that span is the true outage
// window; the leftover duplicates stay unpaired (ongoing). Input order does
// not matter (each scope is re-sorted ascending by created_at, id), and the
// pairing is computed on the unfiltered event set so display filters never
// change a duration. Returns only opener ids; non-incident kinds get no
// entry.
export function pairIncidentDurations(events: AlertEvent[]): Map<number, IncidentDuration> {
  const byScope = new Map<string, AlertEvent[]>()
  for (const e of events) {
    const key = incidentScopeKey(e)
    if (key === null) continue
    const list = byScope.get(key)
    if (list) list.push(e)
    else byScope.set(key, [e])
  }

  const result = new Map<number, IncidentDuration>()
  for (const list of byScope.values()) {
    const ascending = [...list].sort((a, b) => {
      const dt = new Date(a.created_at).getTime() - new Date(b.created_at).getTime()
      return dt !== 0 ? dt : a.id - b.id
    })
    const openDowns: AlertEvent[] = []
    for (const e of ascending) {
      if (isOpener(e.kind)) {
        openDowns.push(e)
        continue
      }
      // A recovery with no open down is a dangling closer — ignored.
      const down = openDowns.shift()
      if (down) {
        const ms = new Date(e.created_at).getTime() - new Date(down.created_at).getTime()
        result.set(down.id, { state: 'paired', ms: Math.max(0, ms), closerId: e.id })
      }
    }
    for (const down of openDowns) {
      result.set(down.id, { state: 'ongoing' })
    }
  }
  return result
}

// One local-calendar-day section of the timeline.
export interface AlertDayGroup {
  key: string // YYYY-MM-DD (local), also the v-for key
  label: string // 今天 · 7 月 31 日 / 昨天 · 7 月 30 日 / 7 月 29 日 / 2025 年 12 月 3 日
  events: AlertEvent[]
}

function localDayKey(d: Date): string {
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
}

// groupEventsByDate buckets events into local calendar days. The input keeps
// its order inside each bucket (the API returns newest first, so the
// timeline reads top-down); day sections are ordered newest first. Labels
// carry the 今天/昨天 anchor relative to `now` plus the date; same-year
// dates omit the year, cross-year dates spell it out.
export function groupEventsByDate(events: AlertEvent[], now: Date): AlertDayGroup[] {
  const todayKey = localDayKey(now)
  const yesterdayKey = localDayKey(new Date(now.getTime() - DAY_MS))

  const groups = new Map<string, AlertDayGroup>()
  for (const e of events) {
    const t = new Date(e.created_at)
    if (Number.isNaN(t.getTime())) continue
    const key = localDayKey(t)
    let group = groups.get(key)
    if (!group) {
      const dateLabel =
        t.getFullYear() === now.getFullYear()
          ? `${t.getMonth() + 1} 月 ${t.getDate()} 日`
          : `${t.getFullYear()} 年 ${t.getMonth() + 1} 月 ${t.getDate()} 日`
      const anchor = key === todayKey ? '今天 · ' : key === yesterdayKey ? '昨天 · ' : ''
      group = { key, label: `${anchor}${dateLabel}`, events: [] }
      groups.set(key, group)
    }
    group.events.push(e)
  }
  // Map preserves insertion order; the API feeds newest-first, so sections
  // are already newest-first — but sort defensively on the day key so a
  // shuffled input cannot scramble the timeline.
  return [...groups.values()].sort((a, b) => (a.key < b.key ? 1 : -1))
}

// --- Inline event detail (GH #144, spec 0019 裁决 4) -------------------------

// Delivery-state vocabulary — the words (成功/失败/未发送) are carried over
// from the history table unchanged. The row cell and the inline detail both
// consume this single source so the two can never fork.
export function alertSentLabel(ev: AlertEvent): string {
  if (ev.kind === 'score_drop_skipped') return '未发送'
  return ev.sent_ok ? '成功' : '失败'
}

// Structured content of one event row's inline expansion (EvalLiveFeed
// precedent — no dialog, no panel). The component renders only; every
// derivation lives here. `pairing` reuses the pairIncidentDurations map
// verbatim (the closer is looked up by its closerId) — a second pairing
// caliber is forbidden by the ticket face.
export interface AlertEventDetail {
  message: string // full, untruncated (the row clamps to two lines)
  timestamp: string // full local reading with date and seconds (formatTime)
  idText: string // 事件 ID #N[ · 端点 ID #M][ · 厂商组 key]
  // Link target for 查看端点详情: the raw endpoint_id, verbatim. A deleted
  // endpoint stays linkable by raw id — the detail page owns the deep-link.
  endpointId: number | null
  pairing:
    | { state: 'paired'; text: string } // 恢复于 … · 持续 …
    | { state: 'ongoing'; text: string } // 进行中
    | { state: 'none'; text: string } // — (non-incident kinds and closers)
  sentText: string // row vocabulary + raw sent_ok flag (投递状态明细)
}

export function buildEventDetail(
  ev: AlertEvent,
  events: AlertEvent[],
  durations: Map<number, IncidentDuration>,
): AlertEventDetail {
  let idText = `事件 ID #${ev.id}`
  if (ev.endpoint_id !== null) idText += ` · 端点 ID #${ev.endpoint_id}`
  if (ev.group_key !== null) idText += ` · 厂商组 ${ev.group_key}`

  const d = durations.get(ev.id)
  let pairing: AlertEventDetail['pairing']
  if (!d) {
    pairing = { state: 'none', text: '—' }
  } else if (d.state === 'ongoing') {
    pairing = { state: 'ongoing', text: '进行中' }
  } else {
    // The closer is guaranteed to live in the same window (the map was built
    // from it); the fallback covers a caller passing a mismatched window.
    const closer = events.find((e) => e.id === d.closerId)
    pairing = {
      state: 'paired',
      text: closer
        ? `恢复于 ${formatTime(closer.created_at)} · 持续 ${formatDuration(d.ms)}`
        : `持续 ${formatDuration(d.ms)}`,
    }
  }

  return {
    message: ev.message,
    timestamp: formatTime(ev.created_at),
    idText,
    endpointId: ev.endpoint_id,
    pairing,
    sentText: `${alertSentLabel(ev)} · sent_ok=${ev.sent_ok ? 'true' : 'false'}`,
  }
}

// --- URL query codec (GH #143, spec 0019 T3 — filter deep-link) ------------

// The /alerts filter state mirrored into the query string: model id, event
// kind, time range. The URL is the shareable form of the exact timeline
// view (modelList.ts five-param codec precedent, 2026-08-02): a pasted link
// must reproduce it, so on open the URL WINS over the defaults.
export interface AlertsFilter {
  model: string | null
  kind: AlertKind | null
  range: AlertTimeRange
}

export const ALERTS_FILTER_DEFAULT: AlertsFilter = { model: null, kind: null, range: '7d' }

const ALERT_TIME_RANGES: AlertTimeRange[] = ['today', '24h', '7d', '30d']

// alertsFilterToQuery serializes only the NON-DEFAULT params — a clean URL
// is the default view (no model / no kind / 7d).
export function alertsFilterToQuery(filter: AlertsFilter): Record<string, string> {
  const query: Record<string, string> = {}
  if (filter.model) query.model = filter.model
  if (filter.kind) query.kind = filter.kind
  if (filter.range !== ALERTS_FILTER_DEFAULT.range) query.range = filter.range
  return query
}

// First value of a possibly-repeated query param (router/index.ts
// firstQueryValue precedent).
function firstQueryString(raw: unknown): string | null {
  const first = Array.isArray(raw) ? raw[0] : raw
  return typeof first === 'string' ? first : null
}

// parseAlertsFilterQuery reads the query back; anything unrecognized falls
// back to the default for that param. The codec is SESSION-BLIND by design:
// a deep link copied from a logged-in operator may carry one of the seven
// kinds an anonymous reader's payload can never contain — parsing it
// verbatim is correct (the empty-result semantics for an unmatchable kind
// is registered on the GH #142 ticket face), not a codec-layer concern.
export function parseAlertsFilterQuery(query: Record<string, unknown>): AlertsFilter {
  const modelRaw = firstQueryString(query.model)
  const kindRaw = firstQueryString(query.kind)
  const rangeRaw = firstQueryString(query.range)
  return {
    model: modelRaw || null,
    kind: ALERT_KINDS.includes(kindRaw as AlertKind) ? (kindRaw as AlertKind) : null,
    range: ALERT_TIME_RANGES.includes(rangeRaw as AlertTimeRange)
      ? (rangeRaw as AlertTimeRange)
      : ALERTS_FILTER_DEFAULT.range,
  }
}
