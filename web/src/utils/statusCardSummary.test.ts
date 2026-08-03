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
  endpointAvailability24h,
  healthyRateRange,
  healthyRangeText,
  healthyRoster,
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

describe('endpointAvailability24h', () => {
  it('is null for an undefined entry (hub-scoped visitor edge, GH #56)', () => {
    expect(endpointAvailability24h(undefined)).toBeNull()
  })
  it('is null when the entry has a fully empty 24h window', () => {
    expect(endpointAvailability24h(makeEntry())).toBeNull()
  })
  it('aggregates probe-weighted and keeps the 95% boundary exact', () => {
    // 19 ok / 20 probes lands exactly on the >=95% "ok" threshold — the KPI
    // must read 0.95, not a drifted window-controlled figure.
    expect(endpointAvailability24h(makeEntry({ dots_24h: dotsWith(20, 1, [23]) }))).toBeCloseTo(0.95)
    // Two probed hours sum across the window: (8 + 90) / (10 + 100) = 0.8909...
    const dots = dotsWith(10, 2, [22])
    dots[23] = { ...dots[23], total: 100, failures: 10 }
    expect(endpointAvailability24h(makeEntry({ dots_24h: dots }))).toBeCloseTo(0.8909, 3)
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

describe('healthyRoster', () => {
  it('keeps only healthy endpoints', () => {
    const roster = healthyRoster([
      makeEntry({ model_id: 'a' }),
      makeEntry({ endpoint_id: 2, model_id: 'b', status: 'degraded' }),
    ])
    expect(roster.rows.map(e => e.model_id)).toEqual(['a'])
    expect(roster.overflow).toBe(0)
  })
  it('sorts by success rate ascending — the most fragile first', () => {
    const roster = healthyRoster([
      makeEntry({ model_id: 'high', success_rate_24h: 1 }),
      makeEntry({ endpoint_id: 2, model_id: 'low', success_rate_24h: 0.96 }),
      makeEntry({ endpoint_id: 3, model_id: 'mid', success_rate_24h: 0.99 }),
    ])
    expect(roster.rows.map(e => e.model_id)).toEqual(['low', 'mid', 'high'])
  })
  it('sinks null rates to the bottom', () => {
    const roster = healthyRoster([
      makeEntry({ model_id: 'no-data', success_rate_24h: null }),
      makeEntry({ endpoint_id: 2, model_id: 'full', success_rate_24h: 1 }),
      makeEntry({ endpoint_id: 3, model_id: 'low', success_rate_24h: 0.9 }),
    ])
    expect(roster.rows.map(e => e.model_id)).toEqual(['low', 'full', 'no-data'])
  })
  it('breaks equal rates by model_id localeCompare (null ties too)', () => {
    const roster = healthyRoster([
      makeEntry({ model_id: 'b-model', success_rate_24h: 0.98 }),
      makeEntry({ endpoint_id: 2, model_id: 'a-model', success_rate_24h: 0.98 }),
      makeEntry({ endpoint_id: 3, model_id: 'z-null', success_rate_24h: null }),
      makeEntry({ endpoint_id: 4, model_id: 'y-null', success_rate_24h: null }),
    ])
    expect(roster.rows.map(e => e.model_id)).toEqual(['a-model', 'b-model', 'y-null', 'z-null'])
  })
  it('caps at 20 rows and reports the overflow', () => {
    const entries = Array.from({ length: 23 }, (_, i) =>
      makeEntry({ endpoint_id: i + 1, model_id: `model-${String(i).padStart(2, '0')}` }),
    )
    const roster = healthyRoster(entries)
    expect(roster.rows).toHaveLength(20)
    expect(roster.overflow).toBe(3)
  })
  it('is empty when no endpoint is healthy (all-abnormal scope)', () => {
    const roster = healthyRoster([makeEntry({ status: 'down' })])
    expect(roster.rows).toEqual([])
    expect(roster.overflow).toBe(0)
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
  it('always lists all four display segments in display order (GH #160)', () => {
    const counts = { ...emptyHealthCounts(), degraded: 3 }
    const segs = distributionSegments(counts)
    expect(segs.map(s => s.status)).toEqual(['stable', 'degraded', 'incident', 'unverified'])
    expect(segs.map(s => s.count)).toEqual([0, 3, 0, 0])
  })
  it('merges down and failing into the incident segment (GH #113)', () => {
    const counts = { ...emptyHealthCounts(), down: 2, failing: 3 }
    const segs = distributionSegments(counts)
    expect(segs.map(s => [s.status, s.label, s.tone, s.count])).toEqual([
      ['stable', '稳定', 'success', 0],
      ['degraded', '降级', 'warning', 0],
      ['incident', '异常', 'danger', 5],
      ['unverified', '未验证', 'neutral', 0],
    ])
  })
  // GH #160: the unverified dimension is never silently omitted — the
  // fourth segment always lists, zero counts included (same disclosure
  // philosophy as the failing alert chip).
  it('discloses unverified in the fourth segment, never merged, never yellow', () => {
    const counts = { ...emptyHealthCounts(), unverified: 2 }
    const segs = distributionSegments(counts)
    const seg = segs.find(s => s.status === 'unverified')
    expect(seg).toEqual({ status: 'unverified', label: '未验证', tone: 'neutral', count: 2 })
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
      '状态全部稳定,但 24h 可用率仅 90.0%,建议持续观察',
    )
  })
  it('declares steady operation only with data backing it', () => {
    const entries = [makeEntry({ dots_24h: dotsWith(100, 0, [23]) })]
    expect(summaryText(emptyHealthCounts(), entries, false)).toBe('近 24 小时运行平稳,无需处理')
  })
  it('never claims 平稳 without probe data', () => {
    const entries = [makeEntry()]
    expect(summaryText(emptyHealthCounts(), entries, false)).toBe('当前全部稳定;暂无 24 小时探测数据')
  })

  // GH #160 (main ruling 2026-08-03): on a STABLE scope containing
  // unverified endpoints the one-liner appends the same neutral note the
  // hero shows (「全部稳定」 never swallows the no-evidence dimension);
  // an abnormal scope's sentence already points at the worst problem, so
  // no note is appended there (the distribution string discloses it).
  it('appends the unverified note on an all-unverified stable scope', () => {
    const entries = [makeEntry({ status: 'unverified' }), makeEntry({ endpoint_id: 2, status: 'unverified' })]
    const counts = { ...emptyHealthCounts(), unverified: 2 }
    expect(summaryText(counts, entries, false)).toBe('当前全部稳定 · 2 个未验证;暂无 24 小时探测数据')
  })
  it('appends the unverified note on a mixed stable scope with data', () => {
    const entries = [
      makeEntry({ dots_24h: dotsWith(100, 0, [23]) }),
      makeEntry({ endpoint_id: 2, status: 'unverified' }),
    ]
    const counts = { ...emptyHealthCounts(), healthy: 1, unverified: 1 }
    expect(summaryText(counts, entries, false)).toBe('近 24 小时运行平稳,无需处理 · 1 个未验证')
  })
  it('appends no note when the scope has abnormal endpoints', () => {
    const entries = [
      makeEntry({ status: 'down', model_id: 'gpt-5.5' }),
      makeEntry({ endpoint_id: 2, status: 'unverified' }),
    ]
    const counts = { ...emptyHealthCounts(), down: 1, unverified: 1 }
    expect(summaryText(counts, entries, false)).toBe('1 个端点异常,建议优先排查 gpt-5.5;暂无 24 小时探测数据')
  })
  it('appends no note when the scope has degraded endpoints', () => {
    const entries = [
      makeEntry({ status: 'degraded', dots_24h: dotsWith(10, 0, [23]) }),
      makeEntry({ endpoint_id: 2, status: 'unverified' }),
    ]
    const counts = { ...emptyHealthCounts(), degraded: 1, unverified: 1 }
    expect(summaryText(counts, entries, false)).toBe('1 个端点降级,建议关注,暂不紧急')
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
      text: '稳定 · 24h 可用率 100.0%',
      tone: 'healthy',
      failingChip: null,
    })
  })
  it('healthy below 95% flags the shortfall', () => {
    const entry = makeEntry({ dots_24h: dotsWith(100, 10, [23]) })
    expect(singleModelStatement(entry, 0.9)).toEqual({
      text: '稳定 · 24h 可用率仅 90.0%,低于 95%',
      tone: 'healthy',
      failingChip: null,
    })
  })
  it('healthy at exactly 95% uses the plain wording', () => {
    const entry = makeEntry({ dots_24h: dotsWith(100, 5, [23]) })
    expect(singleModelStatement(entry, 0.95)).toEqual({
      text: '稳定 · 24h 可用率 95.0%',
      tone: 'healthy',
      failingChip: null,
    })
  })
  it('healthy without probes omits the rate clause', () => {
    expect(singleModelStatement(makeEntry(), null)).toEqual({
      text: '稳定 · 24h 内无探测数据',
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
      text: '异常 · 24h 可用率 0.0%',
      tone: 'abnormal',
      failingChip: null,
    })
  })
  it('failing keeps the double-encoding flag and the abnormal tone', () => {
    const entry = makeEntry({ status: 'failing', dots_24h: dotsWith(100, 30, [23]) })
    expect(singleModelStatement(entry, 0.7)).toEqual({
      text: '异常 · 24h 可用率 70.0%',
      tone: 'abnormal',
      failingChip: '含告警',
    })
  })
  it('down without probes degrades to the no-data clause', () => {
    const entry = makeEntry({ status: 'down' })
    expect(singleModelStatement(entry, null)).toEqual({
      text: '异常 · 24h 内无探测数据',
      tone: 'abnormal',
      failingChip: null,
    })
  })
  // GH #160: unverified is an IN-DOMAIN formal identity — the single-model
  // statement names 未验证 with the neutral tone (Ping endpoints never have
  // probe data, so the no-data clause is the standing form).
  it('gives unverified its formal neutral statement (GH #160)', () => {
    const entry = makeEntry({ status: 'unverified' })
    expect(singleModelStatement(entry, null)).toEqual({
      text: '未验证 · 24h 内无探测数据',
      tone: 'neutral',
      failingChip: null,
    })
  })
  it('keeps the rate clause for an unverified entry with data (defensive shape)', () => {
    const entry = makeEntry({
      status: 'unverified',
      dots_24h: dotsWith(100, 10, [23]),
    })
    expect(singleModelStatement(entry, 0.9)).toEqual({
      text: '未验证 · 24h 可用率 90.0%',
      tone: 'neutral',
      failingChip: null,
    })
  })
  // Defensive fallback (GH #159; narrowed by GH #160): a runtime status
  // OUTSIDE the domain union crashed the share dialog — the switch had no
  // default and the consumers dereferenced undefined. The neutral fallback
  // mirrors statusDisplay's UNKNOWN_DISPLAY: the 未知 word, the middle
  // tone — never stable, never abnormal. unverified used to exercise this
  // branch; post-GH #160 it has its formal identity above, so a made-up
  // out-of-domain value keeps the defense pinned.
  it('returns a neutral statement for a status outside the domain union', () => {
    const entry = makeEntry({ status: 'mystery' as EndpointStatus })
    expect(singleModelStatement(entry, null)).toEqual({
      text: '未知 · 24h 内无探测数据',
      tone: 'degraded',
      failingChip: null,
    })
  })
  it('keeps the rate clause for an unknown status with data', () => {
    const entry = makeEntry({
      status: 'mystery' as EndpointStatus,
      dots_24h: dotsWith(100, 10, [23]),
    })
    expect(singleModelStatement(entry, 0.9)).toEqual({
      text: '未知 · 24h 可用率 90.0%',
      tone: 'degraded',
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
    expect(singleModelSummaryText(makeEntry({ status: 'down' }), 0)).toBe('异常,建议优先排查')
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
      '状态稳定,但 24h 可用率仅 90.0%,建议持续观察',
    )
  })
  it('healthy at or above 95% declares steady operation', () => {
    expect(singleModelSummaryText(makeEntry(), 1)).toBe('近 24 小时运行平稳,无需处理')
  })
  it('healthy without probes states the fact and the gap', () => {
    expect(singleModelSummaryText(makeEntry(), null)).toBe('当前状态稳定;暂无 24 小时探测数据')
  })
  // GH #160: unverified gets its formal single-model summary — states the
  // Ping-monitoring fact without advice (no evidence is neither an alarm nor
  // a comfort). The GH #159 「未知」 defense below stays for out-of-domain
  // runtime values.
  it('unverified states the Ping-monitoring fact, no verdict, no advice', () => {
    expect(singleModelSummaryText(makeEntry({ status: 'unverified' }), null)).toBe(
      '状态未验证:该端点走 Ping 监测,不产生探测记录,暂无健康证据',
    )
  })
  it('unverified keeps the formal wording even with data (constructed case)', () => {
    expect(singleModelSummaryText(makeEntry({ status: 'unverified' }), 0.9)).toBe(
      '状态未验证:该端点走 Ping 监测,不产生探测记录,暂无健康证据',
    )
  })
  // GH #159: an out-of-union runtime status used to fall into the healthy
  // branches and claim "当前状态稳定" — false comfort. The defensive branch
  // states the fact neutrally and never reads as stable.
  it('unknown status without probes never claims stable', () => {
    expect(singleModelSummaryText(makeEntry({ status: 'mystery' as EndpointStatus }), null)).toBe(
      '状态未知;暂无 24 小时探测数据',
    )
  })
  it('unknown status with data states the rate without a verdict', () => {
    expect(singleModelSummaryText(makeEntry({ status: 'mystery' as EndpointStatus }), 0.9)).toBe(
      '状态未知,24h 可用率 90.0%',
    )
  })
})
