// Alert-timeline pure functions (GH #117, spec 0018 §12): time-range
// filtering, down↔recovered duration pairing, and local-date grouping for
// the 故障记录 event timeline. Components render only; every derivation
// lives here so the timeline semantics stay testable (format.ts /
// overviewDots.ts centralization precedent).
import type { AlertEvent } from '@/api/settings'

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
// 'paired' carries the ms span; 'ongoing' means no matching recovery exists
// in the fetched window (the timeline renders 进行中).
export type IncidentDuration = { state: 'paired'; ms: number } | { state: 'ongoing' }

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
        result.set(down.id, { state: 'paired', ms: Math.max(0, ms) })
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
