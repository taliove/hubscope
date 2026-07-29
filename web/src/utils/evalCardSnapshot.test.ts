// Unit tests for buildEvalCardSnapshot (ticket 76, revised to the matrix
// board by ticket 79 / spec 0009). The snapshot carries the EvalCard's
// anti-fake invariant — the scope chips must describe exactly the batch +
// filters + sort + baseline the numbers come from — so chip generation,
// the baseline three-branch copy, the failed warning, the 20-row cap and
// the neutral empty state are pinned down here.
import { describe, it, expect } from 'vitest'
import type { CampaignReport, ReportRow, ReportSuite } from '@/api/types'
import { buildEvalCardSnapshot, EVAL_CARD_MAX_ROWS } from '@/utils/evalCardSnapshot'

function makeSuite(key: string, name: string): ReportSuite {
  return { id: key.length, key, name, version: 1 }
}

function makeRow(modelId: string, overrides: Partial<ReportRow> = {}): ReportRow {
  return {
    model_db_id: 1,
    model_id: modelId,
    family: 'fam-a',
    total_score: 80,
    total_delta: null,
    suite_scores: { reasoning: 80, coding: 70 },
    cells: [],
    ...overrides,
  }
}

function makeReport(overrides: Partial<CampaignReport> = {}): CampaignReport {
  return {
    id: 42,
    trigger: 'manual',
    status: 'done',
    started_at: '2026-07-27T08:00:00Z',
    finished_at: '2026-07-27T09:00:00Z',
    created_at: '2026-07-27T08:00:00Z',
    progress: { total: 4, done: 4, failed: 0, running: 0 },
    suites: [makeSuite('reasoning', '推理'), makeSuite('coding', '编码')],
    weights: { reasoning: 1, coding: 1 },
    rows: [makeRow('model-a'), makeRow('model-b')],
    baseline: null,
    failed_results: 0,
    ...overrides,
  }
}

const defaultQuery = { sort: 'total' }

function chipValues(snapshot: ReturnType<typeof buildEvalCardSnapshot>): Record<string, string> {
  return Object.fromEntries(snapshot.chips.map((c) => [c.label, c.value]))
}

describe('buildEvalCardSnapshot scope chips', () => {
  it('always leads with the neutral batch chip (id · trigger · status)', () => {
    const snapshot = buildEvalCardSnapshot(makeReport(), defaultQuery)
    expect(snapshot.chips[0]).toEqual({ label: '批次', value: '#42 · 手动 · 已完成' })
  })

  it('marks scheduled trigger and failed status in the batch chip', () => {
    const report = makeReport({
      trigger: 'scheduled',
      status: 'failed',
      progress: { total: 4, done: 2, failed: 2, running: 0 },
    })
    const snapshot = buildEvalCardSnapshot(report, defaultQuery)
    expect(snapshot.chips[0]).toEqual({ label: '批次', value: '#42 · 定时 · 失败' })
  })

  it('adds a family chip only when the family filter is active', () => {
    const withFilter = buildEvalCardSnapshot(makeReport(), { family: 'fam-a', sort: 'total' })
    expect(chipValues(withFilter)['系列']).toBe('fam-a')
    const without = buildEvalCardSnapshot(makeReport(), defaultQuery)
    expect(chipValues(without)['系列']).toBeUndefined()
  })

  it('adds a sort chip for any non-default sort (no dimension dedup remains)', () => {
    const snapshot = buildEvalCardSnapshot(makeReport(), { sort: 'coding' })
    expect(chipValues(snapshot)['排序']).toBe('编码')
    expect(chipValues(buildEvalCardSnapshot(makeReport(), defaultQuery))['排序']).toBeUndefined()
  })

  it('never renders a dimension chip (the matrix has no dimension view)', () => {
    const snapshot = buildEvalCardSnapshot(makeReport(), { family: 'fam-a', sort: 'reasoning' })
    expect(chipValues(snapshot)['维度']).toBeUndefined()
  })

  it('keeps the chip order fixed: 批次 → 系列 → 排序 → 涨跌基准', () => {
    const report = makeReport({ baseline: { campaign_id: 41, comparable: true } })
    const snapshot = buildEvalCardSnapshot(report, { family: 'fam-a', sort: 'coding' })
    expect(snapshot.chips.map((c) => c.label)).toEqual(['批次', '系列', '排序', '涨跌基准'])
    const plain = buildEvalCardSnapshot(report, defaultQuery)
    expect(plain.chips.map((c) => c.label)).toEqual(['批次', '涨跌基准'])
  })
})

describe('buildEvalCardSnapshot baseline chip', () => {
  it('names the comparable baseline batch', () => {
    const report = makeReport({ baseline: { campaign_id: 41, comparable: true } })
    const snapshot = buildEvalCardSnapshot(report, defaultQuery)
    expect(chipValues(snapshot)['涨跌基准']).toBe('较批次 #41')
  })

  it.each([
    ['suite_changed', '较批次 #41:题目已变更,分数不可比'],
    ['profile_changed', '较批次 #41:判分口径已变更,分数不可比'],
    ['suite_missing', '较批次 #41:考核口径不同,分数不可比'],
  ])('renders the incomparable reason (%s) with the same words as the page note', (reason, expected) => {
    const report = makeReport({ baseline: { campaign_id: 41, comparable: false, reason } })
    const snapshot = buildEvalCardSnapshot(report, defaultQuery)
    expect(chipValues(snapshot)['涨跌基准']).toBe(expected)
  })

  it('renders no baseline chip without a baseline (first done batch)', () => {
    const snapshot = buildEvalCardSnapshot(makeReport(), defaultQuery)
    expect(chipValues(snapshot)['涨跌基准']).toBeUndefined()
  })
})

describe('buildEvalCardSnapshot failed warning', () => {
  it('carries the page-alert wording for a failed batch', () => {
    const report = makeReport({
      status: 'failed',
      progress: { total: 4, done: 1, failed: 3, running: 0 },
    })
    const snapshot = buildEvalCardSnapshot(report, defaultQuery)
    expect(snapshot.failedWarning).toBe('批次有 3 个评估运行失败,榜单仅统计已完成的评估集')
  })

  it('is null for a done batch', () => {
    const snapshot = buildEvalCardSnapshot(makeReport(), defaultQuery)
    expect(snapshot.failedWarning).toBeNull()
  })
})

describe('buildEvalCardSnapshot rows', () => {
  it('caps the rows at 20 and counts the overflow', () => {
    const rows = Array.from({ length: 25 }, (_, i) => makeRow(`model-${i}`))
    const snapshot = buildEvalCardSnapshot(makeReport({ rows }), defaultQuery)
    expect(snapshot.rows).toHaveLength(EVAL_CARD_MAX_ROWS)
    expect(snapshot.overflowCount).toBe(5)
    expect(snapshot.rows[0].rank).toBe(1)
    expect(snapshot.rows[19].rank).toBe(20)
  })

  it('always reads the total score for the matrix total column', () => {
    const report = makeReport({ rows: [makeRow('model-a')] })
    const snapshot = buildEvalCardSnapshot(report, defaultQuery)
    expect(snapshot.rows[0].score).toBe(80)
  })

  it('shows the delta column only with a comparable baseline', () => {
    const comparable = makeReport({ baseline: { campaign_id: 41, comparable: true } })
    expect(buildEvalCardSnapshot(comparable, defaultQuery).showDeltaColumn).toBe(true)
    const incomparable = makeReport({
      baseline: { campaign_id: 41, comparable: false, reason: 'suite_changed' },
    })
    expect(buildEvalCardSnapshot(incomparable, defaultQuery).showDeltaColumn).toBe(false)
    expect(buildEvalCardSnapshot(makeReport(), defaultQuery).showDeltaColumn).toBe(false)
  })

  it('keeps an empty filtered result neutral (chips stay, no rows, no conclusion)', () => {
    const report = makeReport({ rows: [] })
    const snapshot = buildEvalCardSnapshot(report, { family: 'fam-zzz', sort: 'total' })
    expect(snapshot.rows).toHaveLength(0)
    expect(snapshot.overflowCount).toBe(0)
    expect(chipValues(snapshot)['系列']).toBe('fam-zzz')
    expect(snapshot.failedWarning).toBeNull()
  })

  // Coverage gate (ticket 92, spec 0014 decision A): incomplete rows render
  // the placeholder dash in the material too, with the watermark precomputed
  // (no tooltip fallback in a static image).
  it('nulls the rank and precomputes the watermark for judged-incomplete rows', () => {
    const report = makeReport({
      rows: [
        makeRow('complete-a'),
        makeRow('incomplete-b', {
          total_score: null,
          suite_scores: { reasoning: 80, coding: null },
          complete: false,
          missing_suites: 1,
        }),
      ],
    })
    const snapshot = buildEvalCardSnapshot(report, defaultQuery)
    expect(snapshot.rows[0].rank).toBe(1)
    expect(snapshot.rows[0].incompleteNote).toBe('')
    expect(snapshot.rows[1].rank).toBeNull()
    expect(snapshot.rows[1].incompleteNote).toBe('判分不完整,缺 1/2 维度')
  })
})
