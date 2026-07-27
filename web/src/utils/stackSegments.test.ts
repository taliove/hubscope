// Unit tests for the ScoreStackBar segment pure functions (ticket 75). The
// load-bearing invariant pinned here: segment widths normalize against
// Σw_scored (scored suites only), so the stacked bar length equals the
// backend weighted total under ANY coverage state — no second caliber.
import { describe, it, expect } from 'vitest'
import type { ReportCell, ReportSuite } from '@/api/types'
import {
  LABEL_MIN_PX,
  WATERMARK_EXTRA_PX,
  buildStackSegments,
  effectiveWeight,
} from '@/utils/stackSegments'

function makeSuite(key: string, name = key): ReportSuite {
  return { id: 1, key, name, version: 1 }
}

function makeCell(suiteKey: string, overrides: Partial<ReportCell> = {}): ReportCell {
  return {
    suite_key: suiteKey,
    status: 'done',
    judged_cases: 10,
    expected_cases: 10,
    samples: 30,
    ...overrides,
  }
}

// Reference replica of the backend totalScore
// (internal/server/report_scoring.go): weighted mean over scored suites
// only, weight <=0/missing falls back to 1.
function totalScore(suiteScores: Record<string, number | null>, weights: Record<string, number>): number {
  let sum = 0
  let wsum = 0
  for (const [key, score] of Object.entries(suiteScores)) {
    if (score === null) continue
    const w = weights[key] > 0 ? weights[key] : 1
    sum += w * score
    wsum += w
  }
  return sum / wsum
}

const TRACK = 1000 // px; with widths on the 0-100 scale, px = widthPct x 10

describe('buildStackSegments width normalization', () => {
  it('sums to the total score with equal weights and one unscored suite', () => {
    const suites = ['a', 'b', 'c', 'd', 'e'].map((k) => makeSuite(k))
    const weights = { a: 1, b: 1, c: 1, d: 1, e: 1 }
    const scores = { a: 100, b: 80, c: 60, d: 40, e: null }
    const segments = buildStackSegments(suites, weights, scores, [], TRACK)
    expect(segments.map((s) => s.key)).toEqual(['a', 'b', 'c', 'd'])
    const totalWidth = segments.reduce((acc, s) => acc + s.widthPct, 0)
    expect(totalWidth).toBeCloseTo(totalScore(scores, weights), 10)
    // Each scored suite is 1/4 of the scored weight sum.
    expect(segments[0].widthPct).toBeCloseTo(25, 10)
    expect(segments[3].widthPct).toBeCloseTo(10, 10)
  })

  it('sums to the total score with unequal weights and one unscored suite', () => {
    const suites = ['a', 'b', 'c'].map((k) => makeSuite(k))
    const weights = { a: 2, b: 1, c: 3 }
    const scores = { a: 90, b: 60, c: null }
    const segments = buildStackSegments(suites, weights, scores, [], TRACK)
    const totalWidth = segments.reduce((acc, s) => acc + s.widthPct, 0)
    // total = (90x2 + 60x1) / 3 = 80
    expect(totalScore(scores, weights)).toBeCloseTo(80, 10)
    expect(totalWidth).toBeCloseTo(80, 10)
    expect(segments[0].widthPct).toBeCloseTo(60, 10) // 90 x 2/3
    expect(segments[1].widthPct).toBeCloseTo(20, 10) // 60 x 1/3
  })

  it('keeps suite order and leaves no segment for unscored suites', () => {
    const suites = ['a', 'b', 'c'].map((k) => makeSuite(k))
    const segments = buildStackSegments(suites, { a: 1, b: 1, c: 1 }, { a: null, b: 50, c: null }, [], TRACK)
    expect(segments.map((s) => s.key)).toEqual(['b'])
    expect(segments[0].widthPct).toBeCloseTo(50, 10)
  })

  it('renders an empty segment list when every suite is unscored', () => {
    const suites = ['a', 'b'].map((k) => makeSuite(k))
    expect(buildStackSegments(suites, { a: 1, b: 1 }, { a: null, b: null }, [], TRACK)).toEqual([])
  })

  it('handles a single scored suite', () => {
    const segments = buildStackSegments([makeSuite('a')], { a: 5 }, { a: 42 }, [], TRACK)
    expect(segments).toHaveLength(1)
    expect(segments[0].widthPct).toBeCloseTo(42, 10)
  })

  it('falls back to weight 1 for missing or non-positive weights', () => {
    const suites = ['a', 'b', 'c'].map((k) => makeSuite(k))
    const weights = { a: 0, b: -2 } as Record<string, number> // c missing entirely
    const scores = { a: 90, b: 90, c: 90 }
    const segments = buildStackSegments(suites, weights, scores, [], TRACK)
    for (const s of segments) expect(s.weight).toBe(1)
    const totalWidth = segments.reduce((acc, s) => acc + s.widthPct, 0)
    expect(totalWidth).toBeCloseTo(90, 10)
  })
})

describe('effectiveWeight', () => {
  it('returns the weight when positive', () => {
    expect(effectiveWeight({ a: 2.5 }, 'a')).toBe(2.5)
  })
  it('falls back to 1 for zero, negative and missing weights', () => {
    expect(effectiveWeight({ a: 0 }, 'a')).toBe(1)
    expect(effectiveWeight({ a: -1 }, 'a')).toBe(1)
    expect(effectiveWeight({}, 'a')).toBe(1)
  })
})

describe('coverage watermark and tooltip', () => {
  const suites = [makeSuite('a', '推理')]
  const weights = { a: 1 }
  const scores = { a: 75 }

  it('appends the compressed watermark when a done suite judged fewer cases than expected', () => {
    const cells = [makeCell('a', { judged_cases: 8, expected_cases: 10 })]
    const [seg] = buildStackSegments(suites, weights, scores, cells, TRACK)
    expect(seg.watermark).toBe('·8/10')
    expect(seg.tooltip).toBe('推理 · 75.0 · 判分 8/10 题 · 采样 30 次')
  })

  it('shows no watermark at full coverage or for non-done cells', () => {
    const full = buildStackSegments(suites, weights, scores, [makeCell('a')], TRACK)
    expect(full[0].watermark).toBe('')
    const running = buildStackSegments(suites, weights, scores, [makeCell('a', { status: 'running' })], TRACK)
    expect(running[0].watermark).toBe('')
  })

  it('degrades the tooltip gracefully when the cell is absent', () => {
    const [seg] = buildStackSegments(suites, weights, scores, [], TRACK)
    expect(seg.tooltip).toBe('推理 · 75.0')
  })
})

describe('in-segment label thresholds', () => {
  const suites = [makeSuite('a')]
  const weights = { a: 1 }

  it('shows the label exactly at LABEL_MIN_PX and hides it below', () => {
    // Single suite: widthPct == score. Track 500px -> px = score x 5.
    const at = buildStackSegments(suites, weights, { a: LABEL_MIN_PX / 5 }, [], 500)
    expect(at[0].showLabel).toBe(true)
    const below = buildStackSegments(suites, weights, { a: LABEL_MIN_PX / 5 - 0.2 }, [], 500)
    expect(below[0].showLabel).toBe(false)
  })

  it('shows the watermark only with room for the score plus the suffix', () => {
    const cells = [makeCell('a', { judged_cases: 8, expected_cases: 10 })]
    const needed = LABEL_MIN_PX + WATERMARK_EXTRA_PX
    const wide = buildStackSegments(suites, weights, { a: needed / 5 }, cells, 500)
    expect(wide[0].showWatermark).toBe(true)
    // Wide enough for the label, too narrow for the watermark.
    const mid = buildStackSegments(suites, weights, { a: (needed - 5) / 5 }, cells, 500)
    expect(mid[0].showLabel).toBe(true)
    expect(mid[0].showWatermark).toBe(false)
  })
})
