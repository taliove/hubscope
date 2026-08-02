import { describe, it, expect } from 'vitest'
import {
  healthDeltaText,
  healthDeltaTone,
  heroScopeText,
  emptyStateKind,
  hourlyAvailabilitySeries,
  hourlyProbeSeries,
  hourlyFailureSeries,
  hourlyLatencySeries,
  heroTrendSeries,
  availabilityTierPieces,
  AVAILABILITY_SUCCESS_MIN_100,
  availabilityRateTier,
  abnormalModelCounts,
} from '@/utils/overviewMetrics'
import type { OverviewDot, OverviewEntry } from '@/api/types'

function dot(total: number, failures: number, p50: number | null = null): OverviewDot {
  return { bucket_start: '2026-07-31T00:00:00Z', total, failures, p50_ms: p50 }
}

function entry(partial: Partial<OverviewEntry>): OverviewEntry {
  return {
    endpoint_id: 1,
    model_id: 'm',
    protocol: 'openai',
    enabled: true,
    status: 'healthy',
    status_reason: '',
    degrade_causes: [],
    success_rate_24h: null,
    p50_ms: null,
    p95_ms: null,
    last_probe_at: null,
    family: 'f',
    capability: 'c',
    score: null,
    score_reasons: [],
    dots_24h: [],
    eval_score: null,
    baseline_p50_ms: null,
    ...partial,
  }
}

describe('healthDeltaText', () => {
  it('null renders nothing (no invented trend)', () => {
    expect(healthDeltaText(null)).toBe('')
  })
  it('arrow carries the direction, number is unsigned (GH #129)', () => {
    expect(healthDeltaText(0.012)).toBe('↑ 1.2% 相比昨日')
    expect(healthDeltaText(-0.008)).toBe('↓ 0.8% 相比昨日')
  })
  it('zero is an explicit flat reading', () => {
    expect(healthDeltaText(0)).toBe('相比昨日持平')
  })
})

describe('healthDeltaTone', () => {
  it('gain = success, loss = danger, flat/null = neutral', () => {
    expect(healthDeltaTone(0.01)).toBe('success')
    expect(healthDeltaTone(-0.01)).toBe('danger')
    expect(healthDeltaTone(0)).toBe('neutral')
    expect(healthDeltaTone(null)).toBe('neutral')
  })
})

describe('heroScopeText', () => {
  it('names the enabled-endpoint population', () => {
    expect(heroScopeText(7)).toBe('统计范围：7 个启用端点')
  })
})

describe('hourly series', () => {
  it('availability is per-hour success ratio, null for probe-less hours', () => {
    const s = hourlyAvailabilitySeries([dot(10, 0), dot(10, 5), dot(0, 0)])
    expect(s[0]).toBe(1)
    expect(s[1]).toBe(0.5)
    expect(s[2]).toBeNull()
  })
  it('probe and failure series read the raw counts', () => {
    const dots = [dot(10, 2), dot(0, 0)]
    expect(hourlyProbeSeries(dots)).toEqual([10, 0])
    expect(hourlyFailureSeries(dots)).toEqual([2, 0])
  })
})

describe('hourlyLatencySeries', () => {
  it('weights bucket p50 by successful probes only', () => {
    const entries = [
      entry({ endpoint_id: 1, dots_24h: [dot(10, 0, 100)] }), // 10 successes @100
      entry({ endpoint_id: 2, dots_24h: [dot(10, 5, 300)] }), // 5 successes @300
    ]
    // (100*10 + 300*5) / 15 = 166.67
    expect(hourlyLatencySeries(entries)[0]).toBeCloseTo(166.67, 1)
  })
  it('failed-only buckets and empty windows are null (line breaks)', () => {
    const entries = [entry({ dots_24h: [dot(10, 10, 500), dot(0, 0, null)] })]
    expect(hourlyLatencySeries(entries)).toEqual([null, null])
  })
  it('no entries at all still yields a 24-slot null series', () => {
    expect(hourlyLatencySeries([])).toHaveLength(24)
    expect(hourlyLatencySeries([]).every(v => v === null)).toBe(true)
  })
})

describe('availabilityRateTier', () => {
  it('mirrors the strip thresholds on a rate value', () => {
    expect(availabilityRateTier(1)).toBe('success')
    expect(availabilityRateTier(0.95)).toBe('success')
    expect(availabilityRateTier(0.94)).toBe('warning')
    expect(availabilityRateTier(0)).toBe('danger')
    expect(availabilityRateTier(null)).toBe('none')
  })
})

describe('heroTrendSeries (GH #129)', () => {
  it('lifts the hourly caliber to the 0–100 display scale', () => {
    const s = heroTrendSeries([dot(10, 0), dot(10, 5), dot(4, 4)])
    expect(s.values).toEqual([100, 50, 0])
  })
  it('no-probe hours stay null (the line breaks, never bridges)', () => {
    const s = heroTrendSeries([dot(0, 0), dot(10, 0)])
    expect(s.values).toEqual([null, 100])
  })
  it('labels read the local bucket hour; empty bucket_start yields an empty label', () => {
    const at14 = new Date(2026, 6, 31, 14, 0, 0).toISOString()
    const s = heroTrendSeries([
      { bucket_start: at14, total: 10, failures: 0, p50_ms: null },
      { bucket_start: '', total: 0, failures: 0, p50_ms: null },
    ])
    expect(s.categories).toEqual(['14:00', ''])
  })
  it('an empty dot set yields an empty series', () => {
    expect(heroTrendSeries([])).toEqual({ categories: [], values: [] })
  })
})

describe('availabilityTierPieces (GH #129)', () => {
  const pieces = availabilityTierPieces({ success: 'S', warning: 'W', danger: 'D' })
  it('exactly 0 (probes all failed) is the danger piece', () => {
    expect(pieces[0]).toEqual({ max: 0, color: 'D' })
  })
  it('below the success floor is the warning band', () => {
    expect(pieces[1]).toEqual({ gt: 0, lt: AVAILABILITY_SUCCESS_MIN_100, color: 'W' })
  })
  it('at/above the registered 95 floor is the success piece', () => {
    expect(pieces[2]).toEqual({ gte: 95, color: 'S' })
    expect(AVAILABILITY_SUCCESS_MIN_100).toBe(95)
  })
  it('mirrors the 0~1 sibling caliber (the two scales move together)', () => {
    expect(availabilityRateTier(AVAILABILITY_SUCCESS_MIN_100 / 100)).toBe('success')
    expect(availabilityRateTier((AVAILABILITY_SUCCESS_MIN_100 - 0.1) / 100)).toBe('warning')
    expect(availabilityRateTier(0)).toBe('danger')
  })
})

describe('abnormalModelCounts', () => {
  it('collapses down+failing into incident (display mapping), skips disabled', () => {
    const entries = [
      entry({ endpoint_id: 1, model_id: 'm1', status: 'down' }),
      entry({ endpoint_id: 2, model_id: 'm2', status: 'failing' }),
      entry({ endpoint_id: 3, model_id: 'm3', status: 'degraded' }),
      entry({ endpoint_id: 4, model_id: 'm4', status: 'healthy' }),
      entry({ endpoint_id: 5, model_id: 'm5', status: 'down', enabled: false }),
    ]
    expect(abnormalModelCounts(entries)).toEqual({ incident: 2, degraded: 1, total: 3 })
  })
  it('dedupes by model_id at the worst display state (GH #115 LOW-3 ruling)', () => {
    const entries = [
      entry({ endpoint_id: 1, model_id: 'claude-x', status: 'down' }),
      entry({ endpoint_id: 2, model_id: 'claude-x', status: 'degraded' }),
      entry({ endpoint_id: 3, model_id: 'gpt-y', status: 'degraded' }),
      entry({ endpoint_id: 4, model_id: 'gpt-y', status: 'failing' }),
    ]
    // claude-x counts once (incident wins), gpt-y counts once (incident wins).
    expect(abnormalModelCounts(entries)).toEqual({ incident: 2, degraded: 0, total: 2 })
  })
  it('all-stable is zero', () => {
    expect(abnormalModelCounts([entry({})]).total).toBe(0)
  })
})

// emptyStateKind (GH #159): the dashboard list zone must never render the
// configuration guide ("暂无监控端点") over a failed first load — a load
// failure is not an unconfigured board (失败不冒充空态, ui-guidelines §6).
describe('emptyStateKind', () => {
  it('is none when entries exist (error or not — the board stays on screen)', () => {
    expect(emptyStateKind(3, null, false)).toBe('none')
    expect(emptyStateKind(3, 'boom', true)).toBe('none')
  })
  it('is loading only while the first load is in flight without an error', () => {
    expect(emptyStateKind(0, null, true)).toBe('loading')
  })
  it('is guide only for a clean empty board (loaded, no error, no entries)', () => {
    expect(emptyStateKind(0, null, false)).toBe('guide')
  })
  it('is error when the first load failed — never the guide', () => {
    expect(emptyStateKind(0, 'network down', false)).toBe('error')
  })
  it('keeps the error state while a retry is in flight', () => {
    expect(emptyStateKind(0, 'network down', true)).toBe('error')
  })
})
