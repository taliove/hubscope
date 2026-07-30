// Unit tests for the shared 24h segmented-availability helpers (spec 0017,
// GH #64). aggregateDots24h carries the anti-fake invariant (probe-weighted,
// never a per-endpoint average) and dotTooltipText pins the bucket-fact
// wording both strips render.
import { describe, it, expect } from 'vitest'
import type { OverviewDot, OverviewEntry } from '@/api/types'
import { aggregateDots24h, dotTier, dotTooltipText } from '@/utils/overviewDots'

// Build one enabled entry with overridable fields; 24 empty hourly buckets
// by default so aggregation tests start from a known-zero window.
function makeEntry(overrides: Partial<OverviewEntry> = {}): OverviewEntry {
  return {
    endpoint_id: 1,
    model_id: 'model-a',
    protocol: 'openai',
    enabled: true,
    status: 'healthy',
    status_reason: '',
    degrade_causes: [],
    success_rate_24h: null,
    p50_ms: null,
    p95_ms: null,
    last_probe_at: null,
    family: 'fam',
    capability: 'cap',
    score: null,
    score_reasons: [],
    dots_24h: emptyDots(),
    eval_score: null,
    baseline_p50_ms: null,
    ...overrides,
  }
}

function emptyDots(): OverviewDot[] {
  return Array.from({ length: 24 }, (_, i) => ({
    bucket_start: `2026-07-23T${String(i).padStart(2, '0')}:00:00Z`,
    total: 0,
    failures: 0,
    p50_ms: null,
  }))
}

function dotsWith(total: number, failures: number, hours: number[]): OverviewDot[] {
  const dots = emptyDots()
  for (const h of hours) dots[h] = { ...dots[h], total, failures }
  return dots
}

describe('dotTier', () => {
  it('is none when the hour has no probes', () => {
    expect(dotTier(0, 0)).toBe('none')
  })
  it('is fail when every probe failed', () => {
    expect(dotTier(10, 10)).toBe('fail')
  })
  it('is ok at exactly the 95% boundary', () => {
    expect(dotTier(20, 1)).toBe('ok')
  })
  it('is partial just below 95%', () => {
    expect(dotTier(20, 2)).toBe('partial')
  })
})

describe('aggregateDots24h', () => {
  it('sums totals and failures per hour across entries (mixed scope)', () => {
    const a = makeEntry({ dots_24h: dotsWith(10, 1, [23]) })
    const b = makeEntry({ endpoint_id: 2, dots_24h: dotsWith(30, 9, [23]) })
    const agg = aggregateDots24h([a, b])
    expect(agg).toHaveLength(24)
    expect(agg[23]).toMatchObject({ total: 40, failures: 10 })
    expect(agg[0]).toMatchObject({ total: 0, failures: 0 })
  })
  it('is probe-weighted: a low-traffic healthy endpoint cannot greenwash a high-traffic degraded one', () => {
    // Endpoint A: 10 probes all ok (100%). Endpoint B: 100 probes, 10 failed
    // (90%). A per-endpoint average would read 95% — green at the boundary;
    // the probe-weighted truth is 10/110 failed ≈ 90.9% — yellow. The strip
    // must show the weighted tier (anti-fake invariant).
    const a = makeEntry({ dots_24h: dotsWith(10, 0, [23]) })
    const b = makeEntry({ endpoint_id: 2, dots_24h: dotsWith(100, 10, [23]) })
    const agg = aggregateDots24h([a, b])
    expect(agg[23]).toMatchObject({ total: 110, failures: 10 })
    expect(dotTier(agg[23].total, agg[23].failures)).toBe('partial')
  })
  it('aggregates an all-failed scope to fail tiers', () => {
    const a = makeEntry({ dots_24h: dotsWith(12, 12, [22, 23]) })
    const b = makeEntry({ endpoint_id: 2, dots_24h: dotsWith(8, 8, [23]) })
    const agg = aggregateDots24h([a, b])
    expect(dotTier(agg[22].total, agg[22].failures)).toBe('fail')
    expect(agg[23]).toMatchObject({ total: 20, failures: 20 })
    expect(dotTier(agg[23].total, agg[23].failures)).toBe('fail')
  })
  it('yields 24 empty gray buckets for an empty group', () => {
    const agg = aggregateDots24h([])
    expect(agg).toHaveLength(24)
    expect(agg.every(d => d.total === 0 && d.failures === 0 && d.bucket_start === '')).toBe(true)
  })
  it('keeps the hour alignment: bucket i of the aggregate is bucket i of every entry', () => {
    const a = makeEntry({ dots_24h: dotsWith(5, 0, [3]) })
    const b = makeEntry({ endpoint_id: 2, dots_24h: dotsWith(7, 7, [3]) })
    const agg = aggregateDots24h([a, b])
    for (let i = 0; i < 24; i++) {
      expect(agg[i].bucket_start).toBe(a.dots_24h[i].bucket_start)
    }
    expect(agg[3]).toMatchObject({ total: 12, failures: 7 })
  })
  it('skips entries whose dots window is shorter than the aggregate length', () => {
    const full = makeEntry({ dots_24h: dotsWith(4, 1, [23]) })
    const short = makeEntry({ endpoint_id: 2, dots_24h: [] })
    const agg = aggregateDots24h([full, short])
    expect(agg).toHaveLength(24)
    expect(agg[23]).toMatchObject({ total: 4, failures: 1 })
  })
})

describe('dotTooltipText', () => {
  // Local-time string (no Z suffix) so formatBucketTime's local-time fields
  // are deterministic regardless of the test runner's timezone.
  const dot = (overrides: Partial<OverviewDot>): OverviewDot => ({
    bucket_start: '2026-07-23T14:00:00',
    total: 0,
    failures: 0,
    p50_ms: null,
    ...overrides,
  })

  it('reads the exact success count for an hour with probes', () => {
    expect(dotTooltipText(dot({ total: 40, failures: 10 }))).toBe('07-23 14:00 时段 · 成功 30/40')
  })
  it('reads an all-failed hour as 成功 0/N (bucket fact, never hidden)', () => {
    expect(dotTooltipText(dot({ total: 20, failures: 20 }))).toBe('07-23 14:00 时段 · 成功 0/20')
  })
  it('reads an hour without probes as 无数据', () => {
    expect(dotTooltipText(dot({}))).toBe('07-23 14:00 时段 · 无数据')
  })
  it('drops the time prefix when the aggregate has no bucket start (empty group)', () => {
    expect(dotTooltipText(dot({ bucket_start: '' }))).toBe('无数据')
  })
})
