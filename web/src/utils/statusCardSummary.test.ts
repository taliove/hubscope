// Unit tests for the StatusCard aggregation/copy pure functions (ticket 59).
// These functions carry the card's anti-fake invariant — every number must
// describe exactly the scoped entry set — so the threshold boundaries and
// the summary priority chain are pinned down here.
import { describe, it, expect } from 'vitest'
import type { EndpointStatus, OverviewDot, OverviewEntry } from '@/api/types'
import { emptyHealthCounts } from '@/utils/healthConclusion'
import {
  aggregateDots24h,
  availabilityTier,
  distributionSegments,
  dotTier,
  healthyRateRange,
  healthyRangeText,
  longestDegradedStreak,
  meanP50Ms,
  scopedAvailability,
  singleModelStatement,
  singleModelSummaryText,
  summaryText,
} from '@/utils/statusCardSummary'

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
    ...overrides,
  }
}

function emptyDots(): OverviewDot[] {
  return Array.from({ length: 24 }, (_, i) => ({
    bucket_start: `2026-07-23T${String(i).padStart(2, '0')}:00:00Z`,
    total: 0,
    failures: 0,
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

describe('availabilityTier', () => {
  it('maps null to none', () => {
    expect(availabilityTier(null)).toBe('none')
  })
  it('maps 0% to fail', () => {
    expect(availabilityTier(0)).toBe('fail')
  })
  it('maps exactly 95% to ok', () => {
    expect(availabilityTier(0.95)).toBe('ok')
  })
  it('maps below 95% to partial', () => {
    expect(availabilityTier(0.949)).toBe('partial')
  })
})

describe('aggregateDots24h', () => {
  it('sums totals and failures per hour across entries', () => {
    const a = makeEntry({ dots_24h: dotsWith(10, 1, [23]) })
    const b = makeEntry({ endpoint_id: 2, dots_24h: dotsWith(30, 9, [23]) })
    const agg = aggregateDots24h([a, b])
    expect(agg).toHaveLength(24)
    expect(agg[23]).toMatchObject({ total: 40, failures: 10 })
    expect(agg[0]).toMatchObject({ total: 0, failures: 0 })
  })
  it('yields 24 empty buckets for an empty scope', () => {
    const agg = aggregateDots24h([])
    expect(agg).toHaveLength(24)
    expect(agg.every(d => d.total === 0 && d.failures === 0)).toBe(true)
  })
})

describe('scopedAvailability', () => {
  it('is probe-weighted: a high-traffic outage dominates the rate', () => {
    // Endpoint A: 100 probes, all ok. Endpoint B: 900 probes, all failed.
    // A plain per-endpoint average would read 50%; the probe-weighted truth
    // is 10% — the invariant the card must not fudge.
    const a = makeEntry({ dots_24h: dotsWith(100, 0, [23]) })
    const b = makeEntry({ endpoint_id: 2, dots_24h: dotsWith(900, 900, [23]) })
    expect(scopedAvailability([a, b])).toBeCloseTo(0.1)
  })
  it('is null when nothing was probed', () => {
    expect(scopedAvailability([makeEntry()])).toBeNull()
  })
})

describe('meanP50Ms', () => {
  it('averages entries with data and skips nulls', () => {
    const a = makeEntry({ p50_ms: 100 })
    const b = makeEntry({ endpoint_id: 2, p50_ms: 300 })
    const c = makeEntry({ endpoint_id: 3 })
    expect(meanP50Ms([a, b, c])).toBe(200)
  })
  it('is null when no entry has data', () => {
    expect(meanP50Ms([makeEntry()])).toBeNull()
  })
})

describe('healthyRateRange', () => {
  it('considers only healthy endpoints with data', () => {
    const healthy = makeEntry({ success_rate_24h: 0.98 })
    const degraded = makeEntry({ endpoint_id: 2, status: 'degraded', success_rate_24h: 0.5 })
    const range = healthyRateRange([healthy, degraded])
    expect(range).toEqual({ min: 0.98, max: 0.98 })
  })
  it('is null when no healthy endpoint has data', () => {
    expect(healthyRateRange([makeEntry()])).toBeNull()
  })
})

describe('healthyRangeText', () => {
  it('renders a range when min differs from max', () => {
    const a = makeEntry({ success_rate_24h: 0.982 })
    const b = makeEntry({ endpoint_id: 2, success_rate_24h: 1 })
    expect(healthyRangeText([a, b])).toBe(' · 24h 可用率区间 98.2%–100.0%')
  })
  it('renders a single value when min equals max', () => {
    const a = makeEntry({ success_rate_24h: 1 })
    expect(healthyRangeText([a])).toBe(' · 24h 可用率 100.0%')
  })
  it('appends a spaced no-data note when nothing was probed', () => {
    expect(healthyRangeText([makeEntry()])).toBe(' (24h 内无探测数据)')
  })
})

describe('longestDegradedStreak', () => {
  it('counts continuous non-green hours back from the latest bucket', () => {
    const entry = makeEntry({
      status: 'degraded',
      dots_24h: dotsWith(10, 5, [22, 23]), // last two hours partial
    })
    expect(longestDegradedStreak([entry])).toEqual({ modelId: 'model-a', hours: 2 })
  })
  it('stops at the first green hour', () => {
    const dots = dotsWith(10, 5, [20, 21, 23])
    dots[22] = { ...dots[22], total: 10, failures: 0 } // green hour in between
    const entry = makeEntry({ status: 'degraded', dots_24h: dots })
    expect(longestDegradedStreak([entry])).toEqual({ modelId: 'model-a', hours: 1 })
  })
  it('ignores non-degraded endpoints', () => {
    const entry = makeEntry({ status: 'healthy', dots_24h: dotsWith(10, 10, [23]) })
    expect(longestDegradedStreak([entry])).toBeNull()
  })
  it('breaks the streak at a gray no-data hour (持续 requires evidence)', () => {
    // Latest hour has no probes: the streak never starts, nothing claimed.
    const entry = makeEntry({ status: 'degraded', dots_24h: dotsWith(10, 5, [22]) })
    expect(longestDegradedStreak([entry])).toBeNull()
  })
  it('is null when the latest hour is green', () => {
    const entry = makeEntry({ status: 'degraded', dots_24h: dotsWith(10, 0, [23]) })
    expect(longestDegradedStreak([entry])).toBeNull()
  })
})

describe('distributionSegments', () => {
  it('always lists all four statuses in display order', () => {
    const counts = { ...emptyHealthCounts(), degraded: 3 }
    const segs = distributionSegments(counts)
    expect(segs.map(s => s.status)).toEqual(['healthy', 'degraded', 'down', 'failing'])
    expect(segs.map(s => s.count)).toEqual([0, 3, 0, 0])
  })
})

describe('summaryText', () => {
  const countsOf = (status: EndpointStatus, n: number) => {
    const counts = emptyHealthCounts()
    counts[status] = n
    return counts
  }

  it('returns null for the empty state', () => {
    expect(summaryText(emptyHealthCounts(), [], true)).toBeNull()
  })
  it('prioritizes failing over everything else', () => {
    const entries = [makeEntry({ status: 'failing' }), makeEntry({ endpoint_id: 2, status: 'down' })]
    const counts = { ...emptyHealthCounts(), failing: 1, down: 1 }
    expect(summaryText(counts, entries, false)).toBe('有 1 个端点触发告警,建议立即处理;暂无 24 小时探测数据')
  })
  it('names the first down model', () => {
    const entries = [makeEntry({ status: 'down', model_id: 'gpt-5.5' })]
    expect(summaryText(countsOf('down', 1), entries, false)).toContain('建议优先排查 gpt-5.5')
  })
  it('reports the longest degraded streak when one exists', () => {
    const entries = [
      makeEntry({ status: 'degraded', model_id: 'glm-5.2', dots_24h: dotsWith(10, 5, [21, 22, 23]) }),
    ]
    expect(summaryText(countsOf('degraded', 1), entries, false)).toBe(
      'glm-5.2 持续降级约 3 小时,建议排查上游',
    )
  })
  it('falls back to the plain degraded wording without a streak', () => {
    const entries = [makeEntry({ status: 'degraded', dots_24h: dotsWith(10, 0, [23]) })]
    expect(summaryText(countsOf('degraded', 2), entries, false)).toContain('2 个端点降级,建议关注,暂不紧急')
  })
  it('warns when all green but availability is below 95%', () => {
    const entries = [makeEntry({ dots_24h: dotsWith(100, 10, [23]) })]
    expect(summaryText(emptyHealthCounts(), entries, false)).toBe(
      '状态全部正常,但 24h 可用率仅 90.0%,建议持续观察',
    )
  })
  it('declares steady operation only with data backing it', () => {
    const entries = [makeEntry({ dots_24h: dotsWith(100, 0, [23]) })]
    expect(summaryText(emptyHealthCounts(), entries, false)).toBe('近 24 小时运行平稳,无需处理')
  })
  it('never claims 平稳 without probe data', () => {
    const entries = [makeEntry()]
    expect(summaryText(emptyHealthCounts(), entries, false)).toBe('当前全部正常;暂无 24 小时探测数据')
  })
})

// Single-model mode (design ruling, ticket 60.5 wiring): the card renders one
// endpoint with hubName set, so the conclusion line and the 小结 speak about
// THE endpoint — no counts, no "其余". The anti-fake invariant is unchanged:
// the status word and the failing double-encoding stay front and center.
describe('singleModelStatement', () => {
  it('healthy at or above 95% states the plain rate', () => {
    const entry = makeEntry({ dots_24h: dotsWith(100, 0, [23]) })
    expect(singleModelStatement(entry, 1)).toEqual({
      text: '正常 · 24h 可用率 100.0%',
      tone: 'healthy',
      failingChip: null,
    })
  })
  it('healthy below 95% flags the shortfall', () => {
    const entry = makeEntry({ dots_24h: dotsWith(100, 10, [23]) })
    expect(singleModelStatement(entry, 0.9)).toEqual({
      text: '正常 · 24h 可用率仅 90.0%,低于 95%',
      tone: 'healthy',
      failingChip: null,
    })
  })
  it('healthy at exactly 95% uses the plain wording', () => {
    const entry = makeEntry({ dots_24h: dotsWith(100, 5, [23]) })
    expect(singleModelStatement(entry, 0.95)).toEqual({
      text: '正常 · 24h 可用率 95.0%',
      tone: 'healthy',
      failingChip: null,
    })
  })
  it('healthy without probes omits the rate clause', () => {
    expect(singleModelStatement(makeEntry(), null)).toEqual({
      text: '正常 · 24h 内无探测数据',
      tone: 'healthy',
      failingChip: null,
    })
  })
  it('degraded states the status word and the rate', () => {
    const entry = makeEntry({ status: 'degraded', dots_24h: dotsWith(100, 20, [23]) })
    expect(singleModelStatement(entry, 0.8)).toEqual({
      text: '降级 · 24h 可用率 80.0%',
      tone: 'degraded',
      failingChip: null,
    })
  })
  it('down states the status word and the rate', () => {
    const entry = makeEntry({ status: 'down', dots_24h: dotsWith(100, 100, [23]) })
    expect(singleModelStatement(entry, 0)).toEqual({
      text: '宕机 · 24h 可用率 0.0%',
      tone: 'abnormal',
      failingChip: null,
    })
  })
  it('failing keeps the double-encoding flag and the abnormal tone', () => {
    const entry = makeEntry({ status: 'failing', dots_24h: dotsWith(100, 30, [23]) })
    expect(singleModelStatement(entry, 0.7)).toEqual({
      text: '告警 · 24h 可用率 70.0%',
      tone: 'abnormal',
      failingChip: '含告警',
    })
  })
  it('down without probes degrades to the no-data clause', () => {
    const entry = makeEntry({ status: 'down' })
    expect(singleModelStatement(entry, null)).toEqual({
      text: '宕机 · 24h 内无探测数据',
      tone: 'abnormal',
      failingChip: null,
    })
  })
})

describe('singleModelSummaryText', () => {
  it('failing tells the reader to act now', () => {
    expect(singleModelSummaryText(makeEntry({ status: 'failing' }), null)).toBe(
      '触发告警,建议立即处理;暂无 24 小时探测数据',
    )
  })
  it('down names no model (the card scope already does)', () => {
    expect(singleModelSummaryText(makeEntry({ status: 'down' }), 0)).toBe('宕机,建议优先排查')
  })
  it('reports the degraded streak when one exists', () => {
    const entry = makeEntry({ status: 'degraded', dots_24h: dotsWith(10, 5, [21, 22, 23]) })
    expect(singleModelSummaryText(entry, 0.5)).toBe('持续降级约 3 小时,建议排查上游')
  })
  it('degraded without a streak stays low-urgency', () => {
    const entry = makeEntry({ status: 'degraded', dots_24h: dotsWith(10, 0, [23]) })
    expect(singleModelSummaryText(entry, 1)).toBe('降级,建议关注,暂不紧急')
  })
  it('healthy below 95% suggests watching', () => {
    expect(singleModelSummaryText(makeEntry(), 0.9)).toBe(
      '状态正常,但 24h 可用率仅 90.0%,建议持续观察',
    )
  })
  it('healthy at or above 95% declares steady operation', () => {
    expect(singleModelSummaryText(makeEntry(), 1)).toBe('近 24 小时运行平稳,无需处理')
  })
  it('healthy without probes states the fact and the gap', () => {
    expect(singleModelSummaryText(makeEntry(), null)).toBe('当前状态正常;暂无 24 小时探测数据')
  })
})
