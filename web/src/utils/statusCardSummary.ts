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
import type { OverviewDot, OverviewEntry } from '@/api/types'
import { formatPercent, formatPercentDigits } from '@/utils/format'
import { STATUS_LABELS, type HealthCounts } from '@/utils/healthConclusion'

// Availability tiers (ui-guidelines §3): same thresholds as EndpointCard
// dots — no data gray, 0% red, ≥95% green, below 95% yellow.
export type AvailabilityTier = 'none' | 'fail' | 'partial' | 'ok'

export function dotTier(total: number, failures: number): AvailabilityTier {
  if (total === 0) return 'none'
  if (failures >= total) return 'fail'
  return (total - failures) / total >= 0.95 ? 'ok' : 'partial'
}

export function availabilityTier(rate: number | null): AvailabilityTier {
  if (rate === null) return 'none'
  if (rate <= 0) return 'fail'
  return rate >= 0.95 ? 'ok' : 'partial'
}

// Sum per-hour totals/failures across entries, oldest hour first. This is
// probe-weighted by construction — a plain per-endpoint average would let a
// low-traffic endpoint hide a high-traffic outage.
export function aggregateDots24h(entries: OverviewEntry[]): OverviewDot[] {
  const withDots = entries.find(e => e.dots_24h.length > 0)
  const length = withDots ? withDots.dots_24h.length : 24
  const out: OverviewDot[] = []
  for (let i = 0; i < length; i++) {
    let total = 0
    let failures = 0
    for (const entry of entries) {
      const dot = entry.dots_24h[i]
      if (!dot) continue
      total += dot.total
      failures += dot.failures
    }
    out.push({ bucket_start: withDots?.dots_24h[i]?.bucket_start ?? '', total, failures })
  }
  return out
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

// Healthy-side summary line under the abnormal list: range when data exists,
// a no-data note otherwise. Single value when min == max.
export function healthyRangeText(entries: OverviewEntry[]): string {
  const range = healthyRateRange(entries)
  if (range === null) return ' (24h 内无探测数据)'
  if (range.min === range.max) return ` · 24h 可用率 ${formatPercent(range.min)}`
  return ` · 24h 可用率区间 ${formatPercentDigits(range.min)}%–${formatPercentDigits(range.max)}%`
}
