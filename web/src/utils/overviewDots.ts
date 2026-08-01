// Shared 24h segmented-availability helpers (spec 0017, GH #64): the single
// source for the per-bucket tier mapping, the probe-weighted group aggregate,
// and the slot tooltip wording. Current consumer — the share-card material
// (statusCardSummary re-export; the model-list row micro strip retired with
// GH #131, its trend column now a latency sparkline) — derives coloring and
// tooltip text from these functions so all strips are consistent by
// construction, not by convention.
//
// Anti-fake invariant (ui-guidelines §3 batch-59 registration): aggregation
// is probe-weighted per-hour sums of total/failures, NEVER a per-endpoint
// average — a low-traffic endpoint must not hide a high-traffic outage. This
// is identical to the backend figure by construction (same definition as
// internal/server/overview.go's groupAccumulator).
import type { OverviewDot, OverviewEntry } from '@/api/types'
import { formatBucketTime } from '@/utils/format'

// Availability tiers (ui-guidelines §3): same thresholds as EndpointCard
// dots — no data gray, 0% red, ≥95% green, below 95% yellow.
export type AvailabilityTier = 'none' | 'fail' | 'partial' | 'ok'

export function dotTier(total: number, failures: number): AvailabilityTier {
  if (total === 0) return 'none'
  if (failures >= total) return 'fail'
  return (total - failures) / total >= 0.95 ? 'ok' : 'partial'
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
    // Aggregated dots carry counts only: a cross-endpoint P50 cannot be
    // derived from per-bucket percentiles, so p50_ms stays null (the
    // segmented availability bar never feeds the sparkline).
    out.push({ bucket_start: withDots?.dots_24h[i]?.bucket_start ?? '', total, failures, p50_ms: null })
  }
  return out
}

// Slot tooltip wording, isomorphic to the EndpointCard dots (ui-guidelines
// §5 LatencySparkline bucket-fact precedent): an hour with probes reads the
// exact success count, an hour without probes reads 无数据. An aggregate of
// an empty entry set has no bucket_start at all — the tooltip then drops the
// time prefix rather than rendering a blank one.
export function dotTooltipText(dot: OverviewDot): string {
  const label = dot.bucket_start ? `${formatBucketTime(dot.bucket_start)} 时段` : ''
  if (dot.total === 0) return label ? `${label} · 无数据` : '无数据'
  return `${label} · 成功 ${dot.total - dot.failures}/${dot.total}`
}
