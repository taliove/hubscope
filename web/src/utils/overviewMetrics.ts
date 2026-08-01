// Overview hero + metric-widget derivations (GH #115, spec 0018 §6/§7):
// every displayed figure is either a backend aggregate rendered verbatim
// (health index, delta, probes, availability) or a display-only trend
// series derived here — centralized as pure functions so components never
// carry math, and every wording lives in one place.
//
// Anti-fake invariants carried over from the old board:
//   - The health index is NEVER fabricated: null renders 暂无数据, never
//     100% (spec 0018 user story 17).
//   - Hourly aggregation is probe-weighted (overviewDots.ts discipline):
//     a low-traffic endpoint must not hide a high-traffic outage.
//   - The delta is rendered only when the backend provides it (both
//     windows have data); a null delta renders NOTHING — no invented
//     "flat" trend.
import type { OverviewDot, OverviewEntry } from '@/api/types'
import { toDisplayStatus } from '@/utils/statusDisplay'

// --- Hero wording ---------------------------------------------------------

// Day-over-day delta wording: 「较昨日 +1.2%」 / 「较昨日 -0.8%」 /
// 「较昨日持平」; null → '' (the hero hides the delta line entirely).
// The delta is a difference of two 0~1 ratios, rendered in percentage
// points with one decimal.
export function healthDeltaText(delta: number | null): string {
  if (delta === null || delta === undefined) return ''
  const points = delta * 100
  if (Math.abs(points) < 0.05) return '较昨日持平'
  return `较昨日 ${points > 0 ? '+' : ''}${points.toFixed(1)}%`
}

// Tone of the delta line: an availability gain is good (success), a loss is
// bad (danger), flat is neutral. Null never reaches here (the line hides).
export function healthDeltaTone(delta: number | null): 'success' | 'danger' | 'neutral' {
  if (delta === null || delta === undefined) return 'neutral'
  const points = delta * 100
  if (Math.abs(points) < 0.05) return 'neutral'
  return points > 0 ? 'success' : 'danger'
}

// Statistical-scope annotation (spec 0018 user story 16): the hero figure
// always names its population so it can never read as a partial sample.
export function heroScopeText(enabledEndpoints: number): string {
  return `统计范围：${enabledEndpoints} 个启用端点`
}

// --- Widget hourly trend series -------------------------------------------
// Display-only sparkline inputs; the scalars beside them always come from
// backend aggregates. All series are hour-aligned, oldest first.

// Hourly availability of the aggregated dots: 1 − failures/total per hour;
// null for hours without probes (the sparkline breaks there).
export function hourlyAvailabilitySeries(dots: OverviewDot[]): (number | null)[] {
  return dots.map(d => (d.total === 0 ? null : (d.total - d.failures) / d.total))
}

// Hourly probe totals (request-volume sparkline).
export function hourlyProbeSeries(dots: OverviewDot[]): number[] {
  return dots.map(d => d.total)
}

// Hourly failure counts (abnormal-widget sparkline — the global "几点开始
// 炸的" shape).
export function hourlyFailureSeries(dots: OverviewDot[]): number[] {
  return dots.map(d => d.failures)
}

// Hourly mean latency across entries, weighted by each bucket's SUCCESSFUL
// probes (failed-probe latency is time-to-failure and never counts — the
// LatencySparkline discipline). null for hours with no successful probe.
export function hourlyLatencySeries(entries: OverviewEntry[]): (number | null)[] {
  const length = entries.find(e => e.dots_24h.length > 0)?.dots_24h.length ?? 24
  const out: (number | null)[] = []
  for (let i = 0; i < length; i++) {
    let weighted = 0
    let successes = 0
    for (const entry of entries) {
      const dot = entry.dots_24h[i]
      if (!dot || dot.p50_ms === null) continue
      const ok = dot.total - dot.failures
      if (ok <= 0) continue
      weighted += dot.p50_ms * ok
      successes += ok
    }
    out.push(successes > 0 ? weighted / successes : null)
  }
  return out
}

// --- Row-level derivations --------------------------------------------------

// Text tier of a 24h availability RATE (0~1): the same thresholds as the
// segmented-strip tiers (ui-guidelines §3 registration, carried into v2) —
// ≥95% success, below warning, 0% with data danger, null no tier. Text
// scenarios consume the *-text grade of the tier's slot.
export type RateTier = 'success' | 'warning' | 'danger' | 'none'

export function availabilityRateTier(rate: number | null): RateTier {
  if (rate === null || rate === undefined) return 'none'
  if (rate === 0) return 'danger'
  return rate >= 0.95 ? 'success' : 'warning'
}

// --- Abnormal-model count ---------------------------------------------------

export interface AbnormalCounts {
  incident: number
  degraded: number
  total: number
}

// Enabled MODELS whose DISPLAY state is not stable, deduplicated by
// model_id (main ruling, GH #115 check LOW-3): the widget answers "which
// models are at risk", so a model with two abnormal protocols still counts
// once, at its WORST display state (display-layer mapping: down + failing
// count together as 异常). Disabled endpoints are out of service by
// admin choice and never inflate the abnormal count.
export function abnormalModelCounts(entries: OverviewEntry[]): AbnormalCounts {
  const worstByModel = new Map<string, 'incident' | 'degraded'>()
  for (const entry of entries) {
    if (!entry.enabled) continue
    const display = toDisplayStatus(entry.status)
    if (display !== 'incident' && display !== 'degraded') continue
    const current = worstByModel.get(entry.model_id)
    if (current === 'incident') continue
    if (display === 'incident' || current === undefined) worstByModel.set(entry.model_id, display)
  }
  let incident = 0
  let degraded = 0
  for (const display of worstByModel.values()) {
    if (display === 'incident') incident++
    else degraded++
  }
  return { incident, degraded, total: incident + degraded }
}
