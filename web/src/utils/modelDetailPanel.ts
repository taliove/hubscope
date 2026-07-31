// Pure derivations for the model detail side panel (GH #116, spec 0018 §10).
//
// Two calibers live here, centralized so the component only renders:
//   1. Panel metrics — every scalar derives from the FROZEN entry snapshot
//      taken at row-click time. No second fetch feeds the metric cells, so
//      the panel and the originating list row can never disagree, and the
//      overview poll (10s) cannot mutate an open panel.
//   2. Failure events — the event-record section is derived from the same
//      one-time 24h probe fetch that feeds the latency chart (a single
//      async region, a single failure surface).
import type { OverviewEntry, ProbeRecord } from '@/api/types'

export interface PanelMetrics {
  availability: number | null // 0~1
  errorRate: number | null // 0~1
  latencyP50Ms: number | null
  latencyP95Ms: number | null
}

// Metric scalars of the panel header block. The error rate is the exact
// complement of the 24h availability — never an independently computed
// figure (a second caliber could disagree with the availability cell by
// rounding alone). Null stays null: a no-probe window renders dashes,
// never an invented 0% error rate (anti-fake).
export function panelMetrics(entry: OverviewEntry): PanelMetrics {
  return {
    availability: entry.success_rate_24h,
    errorRate: entry.success_rate_24h === null ? null : 1 - entry.success_rate_24h,
    latencyP50Ms: entry.p50_ms,
    latencyP95Ms: entry.p95_ms,
  }
}

export interface FailureEvent {
  id: number
  createdAt: string
  streaming: boolean
  reason: string
}

// How many failure events the panel lists; the rest stays on the full
// detail page (the panel is a triage surface, not the archive).
export const FAILURE_EVENT_LIMIT = 8

// Recent failure events, newest first, capped at `limit`. The API order is
// NOT relied on — records are sorted defensively by created_at desc with an
// id tiebreak (two probes can share a timestamp). A null/blank
// error_summary falls back to a neutral word rather than rendering empty —
// the row must always say WHY it exists.
export function recentFailureEvents(records: ProbeRecord[], limit = FAILURE_EVENT_LIMIT): FailureEvent[] {
  return records
    .filter(r => !r.ok)
    .slice() // never mutate the caller's array
    .sort((a, b) => b.created_at.localeCompare(a.created_at) || b.id - a.id)
    .slice(0, limit)
    .map(r => ({
      id: r.id,
      createdAt: r.created_at,
      streaming: r.streaming,
      reason: r.error_summary && r.error_summary.trim() !== '' ? r.error_summary : '未知错误',
    }))
}
