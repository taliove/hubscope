// Aggregation and copy logic for the StatusCard share material (ticket 59,
// ui-guidelines §5 batch-59 registration). Pure functions only so the card
// component stays presentational.
//
// Scope consistency is the anti-fake invariant (mirror of ADR 0007): every
// number on the card is computed from the SAME enabled-entry set the scope
// chips describe. Group/global backend aggregates are NOT passed through —
// they describe the unfiltered scope and would contradict the chips whenever
// a keyword/protocol/status filter is active (and the global overview has no
// latency aggregate at all). The dots-based availability below is identical
// to the backend figure by construction (probe-weighted ok/total, same as
// internal/server/overview.go's groupAccumulator).
import type { OverviewEntry } from '@/api/types'
import { formatPercent, formatPercentDigits } from '@/utils/format'
import { STATUS_LABELS, type HealthCounts, type HealthTone } from '@/utils/healthConclusion'
import { dotTier, type AvailabilityTier } from '@/utils/overviewDots'

// dotTier / aggregateDots24h / AvailabilityTier live in utils/overviewDots.ts
// since spec 0017 (GH #64) so the group-level UptimeStrip shares the batch-59
// probe-weighted aggregation by construction; re-exported here to keep the
// existing import sites (StatusCard* components, EndpointDetailView) stable.
export { aggregateDots24h, dotTier } from '@/utils/overviewDots'
export type { AvailabilityTier } from '@/utils/overviewDots'

export function availabilityTier(rate: number | null): AvailabilityTier {
  if (rate === null) return 'none'
  if (rate <= 0) return 'fail'
  return rate >= 0.95 ? 'ok' : 'partial'
}

// Probe-weighted availability of the whole window; null when nothing probed.
export function scopedAvailability(entries: OverviewEntry[]): number | null {
  let total = 0
  let ok = 0
  for (const entry of entries) {
    for (const dot of entry.dots_24h) {
      total += dot.total
      ok += dot.total - dot.failures
    }
  }
  return total === 0 ? null : ok / total
}

// 24h availability of a single endpoint from its overview entry (GH #56,
// EndpointDetail KPI). An undefined entry (a hub-scoped visitor cannot see a
// foreign hub's entry in the overview payload) degrades to null — the KPI
// renders a neutral no-data placeholder and must NOT fall back to the chart
// buckets, which follow the window/mode controls and would let the KPI drift.
export function endpointAvailability24h(entry: OverviewEntry | undefined): number | null {
  if (!entry) return null
  return scopedAvailability([entry])
}

// Mean of per-endpoint p50 latencies across entries with data; null when
// none. Scope-consistent by construction (see the module header for why the
// backend group aggregate is not reused here).
export function meanP50Ms(entries: OverviewEntry[]): number | null {
  const values = entries.filter(e => e.p50_ms !== null).map(e => e.p50_ms as number)
  if (values.length === 0) return null
  return values.reduce((sum, v) => sum + v, 0) / values.length
}

// 24h success-rate range among healthy endpoints; null when none have data.
export function healthyRateRange(entries: OverviewEntry[]): { min: number; max: number } | null {
  const values = entries
    .filter(e => e.status === 'healthy' && e.success_rate_24h !== null)
    .map(e => e.success_rate_24h as number)
  if (values.length === 0) return null
  return { min: Math.min(...values), max: Math.max(...values) }
}

// Healthy-endpoint roster (GH #92, design ruling on the ticket; share-materials
// surface brief): the compact name-list that replaces the batch-59 "其余 N 个
// 端点正常 · 24h 可用率区间" summary line. Listing EVERY healthy endpoint (not
// just parading the abnormal ones) is an anti-fake strengthening — the roster
// is the superset of the old range text.
// Sort: 24h success rate ASCENDING (the most fragile healthy endpoint first —
// visual weight follows business severity), null rates sink to the bottom,
// ties break by model_id localeCompare. Capped so a large group cannot produce
// an unbounded tall image (same philosophy as the abnormal list's cap of 10);
// the footer origin is the escape hatch to the live board.
export const HEALTHY_ROSTER_CAP = 20 // 2 columns x 10 rows

export interface HealthyRoster {
  rows: OverviewEntry[]
  overflow: number // healthy endpoints not listed (rows capped)
}

export function healthyRoster(entries: OverviewEntry[]): HealthyRoster {
  const healthy = entries
    .filter(e => e.status === 'healthy')
    .sort((a, b) => {
      if (a.success_rate_24h === null && b.success_rate_24h === null) {
        return a.model_id.localeCompare(b.model_id)
      }
      if (a.success_rate_24h === null) return 1 // null sinks
      if (b.success_rate_24h === null) return -1
      return a.success_rate_24h - b.success_rate_24h || a.model_id.localeCompare(b.model_id)
    })
  const rows = healthy.slice(0, HEALTHY_ROSTER_CAP)
  return { rows, overflow: healthy.length - rows.length }
}

// Longest continuous degraded streak (hours) counting back from the latest
// bucket, among degraded endpoints. Only hours WITH probes that came back
// non-green count; a gray no-data hour breaks the streak — "持续" requires
// continuous evidence, otherwise any sparse-data endpoint would inflate to
// "约 24 小时" and the summary would cry wolf. Returns null when no degraded
// endpoint has a streak.
export function longestDegradedStreak(entries: OverviewEntry[]): { modelId: string; hours: number } | null {
  let best: { modelId: string; hours: number } | null = null
  for (const entry of entries) {
    if (entry.status !== 'degraded') continue
    let hours = 0
    for (let i = entry.dots_24h.length - 1; i >= 0; i--) {
      const dot = entry.dots_24h[i]
      const tier = dotTier(dot.total, dot.failures)
      if (tier === 'ok' || tier === 'none') break
      hours += 1
    }
    if (hours > 0 && (best === null || hours > best.hours)) {
      best = { modelId: entry.model_id, hours }
    }
  }
  return best
}

// One-sentence summary (ui-guidelines §5 StatusCard 一句话总结): first match
// wins, ordered by severity. While any endpoint is abnormal the sentence
// must point at the worst problem — never a "运行平稳" phrasing (anti-fake
// at the copy layer). Returns null for the empty state (no summary row).
export function summaryText(counts: HealthCounts, entries: OverviewEntry[], empty: boolean): string | null {
  if (empty) return null
  const availability = scopedAvailability(entries)
  let text: string
  if (counts.failing > 0) {
    text = `有 ${counts.failing} 个端点触发告警,建议立即处理`
  } else if (counts.down > 0) {
    const first = entries.find(e => e.status === 'down')
    text = first
      ? `${counts.down} 个端点宕机,建议优先排查 ${first.model_id}`
      : `${counts.down} 个端点宕机,建议优先排查`
  } else if (counts.degraded > 0) {
    const streak = longestDegradedStreak(entries)
    text = streak
      ? `${streak.modelId} 持续降级约 ${streak.hours} 小时,建议排查上游`
      : `${counts.degraded} 个端点降级,建议关注,暂不紧急`
  } else if (availability !== null && availability < 0.95) {
    text = `状态全部正常,但 24h 可用率仅 ${formatPercent(availability)},建议持续观察`
  } else if (availability !== null) {
    text = '近 24 小时运行平稳,无需处理'
  } else {
    // All green but the window has no probes: "平稳" would claim evidence we
    // do not have, so state the fact and the gap.
    text = '当前全部正常'
  }
  if (availability === null) text += ';暂无 24 小时探测数据'
  return text
}

// --- Single-model mode (design ruling, ticket 60.5 wiring) -----------------
// A single-model card (entries.length === 1 && hubName set) renders its hero
// and summary with these two functions instead of the aggregate versions:
// the conclusion speaks about THE endpoint (no counts, no distribution), the
// failing double-encoding (orange dot + chip) is preserved, and the summary
// never papers over an abnormal state — same anti-fake invariant, one model.

export interface SingleModelStatement {
  text: string
  tone: HealthTone // reuses the vc-healthy / vc-degraded / vc-abnormal classes
  // Failing double-encoding (design ruling): the static orange chip copy,
  // '含告警' when the endpoint is failing, null otherwise — the template
  // renders the chip (with the orange dot) only when this is non-null.
  failingChip: string | null
}

// Statement under the availability number, replacing the aggregate verdict +
// distribution: "降级 · 24h 可用率 80.0%". The rate clause degrades to a
// no-data note when the window has no probes.
export function singleModelStatement(entry: OverviewEntry, availability: number | null): SingleModelStatement {
  const rate = availability !== null ? `24h 可用率 ${formatPercent(availability)}` : '24h 内无探测数据'
  switch (entry.status) {
    case 'healthy':
      return {
        text:
          availability !== null && availability < 0.95
            ? `正常 · 24h 可用率仅 ${formatPercent(availability)},低于 95%`
            : `正常 · ${rate}`,
        tone: 'healthy',
        failingChip: null,
      }
    case 'degraded':
      return { text: `降级 · ${rate}`, tone: 'degraded', failingChip: null }
    case 'down':
      return { text: `宕机 · ${rate}`, tone: 'abnormal', failingChip: null }
    case 'failing':
      return { text: `告警 · ${rate}`, tone: 'abnormal', failingChip: '含告警' }
  }
}

// Single-model one-sentence summary: same priority chain as the aggregate
// summaryText, with singular phrasing (no counts, no model name — the scope
// chips already name the model).
export function singleModelSummaryText(entry: OverviewEntry, availability: number | null): string {
  let text: string
  if (entry.status === 'failing') {
    text = '触发告警,建议立即处理'
  } else if (entry.status === 'down') {
    text = '宕机,建议优先排查'
  } else if (entry.status === 'degraded') {
    const streak = longestDegradedStreak([entry])
    text = streak ? `持续降级约 ${streak.hours} 小时,建议排查上游` : '降级,建议关注,暂不紧急'
  } else if (availability !== null && availability < 0.95) {
    text = `状态正常,但 24h 可用率仅 ${formatPercent(availability)},建议持续观察`
  } else if (availability !== null) {
    text = '近 24 小时运行平稳,无需处理'
  } else {
    text = '当前状态正常'
  }
  if (availability === null) text += ';暂无 24 小时探测数据'
  return text
}

// Distribution segment of the conclusion block: all four statuses always
// listed (zero counts included) so "no failing" is confirmed at a glance
// rather than inferred from absence.
export interface DistributionSegment {
  status: keyof HealthCounts
  label: string
  count: number
}

export function distributionSegments(counts: HealthCounts): DistributionSegment[] {
  return (['healthy', 'degraded', 'down', 'failing'] as const).map(status => ({
    status,
    label: STATUS_LABELS[status],
    count: counts[status],
  }))
}

// Healthy-side 24h availability suffix; null when nothing was probed. Single
// value when min == max. GH #92: the aggregate usage ("其余 N 个端点正常" line
// and the all-healthy range suffix) retired with the healthy roster — the
// per-entry colored rates are a superset of the range text. The single-model
// line keeps it (one entry, always the single-value form).
export function healthyRangeText(entries: OverviewEntry[]): string {
  const range = healthyRateRange(entries)
  if (range === null) return ' (24h 内无探测数据)'
  if (range.min === range.max) return ` · 24h 可用率 ${formatPercent(range.min)}`
  return ` · 24h 可用率区间 ${formatPercentDigits(range.min)}%–${formatPercentDigits(range.max)}%`
}
