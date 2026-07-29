// Unit tests for the shared eval-board wording (ticket 76 check follow-up).
// These strings have multiple consumers (page alerts, the toolbar note and
// the EvalCard), so the verbatim copy is pinned here against drift.
import { describe, it, expect } from 'vitest'
import type { EvalTrigger, ReportRow } from '@/api/types'
import {
  failedBatchWarning,
  baselineNoteText,
  baselineChipText,
  campaignSettleVerb,
  campaignTriggerLabel,
  incompleteWatermark,
} from '@/utils/evalWording'

describe('campaignTriggerLabel', () => {
  it.each([
    ['scheduled', '定时'],
    ['manual', '手动'],
  ] as [EvalTrigger, string][])('maps %s verbatim', (trigger, expected) => {
    expect(campaignTriggerLabel(trigger)).toBe(expected)
  })

  it('falls back to the raw value for an unknown trigger', () => {
    expect(campaignTriggerLabel('webhook' as EvalTrigger)).toBe('webhook')
  })
})

describe('campaignSettleVerb', () => {
  it('reads 失败于 for a failed batch (anti-fake: never reads as completed)', () => {
    expect(campaignSettleVerb('failed')).toBe('失败于')
  })

  it('reads 完成于 otherwise (board batches are always settled)', () => {
    expect(campaignSettleVerb('done')).toBe('完成于')
    expect(campaignSettleVerb('running')).toBe('完成于')
  })
})

describe('failedBatchWarning', () => {
  it('matches the page alert title verbatim', () => {
    expect(failedBatchWarning(3)).toBe('批次有 3 个评估运行失败,榜单仅统计已完成的评估集')
  })
})

describe('baselineNoteText', () => {
  it('is empty without a baseline (first done batch)', () => {
    expect(baselineNoteText(null)).toBe('')
  })

  it('names the comparable baseline batch', () => {
    expect(baselineNoteText({ campaign_id: 41, comparable: true })).toBe('涨跌较批次 #41')
  })

  it.each([
    ['suite_changed', '较批次 #41:题目已变更,分数不可比'],
    ['profile_changed', '较批次 #41:判分口径已变更,分数不可比'],
    ['suite_missing', '较批次 #41:考核口径不同,分数不可比'],
  ])('renders the incomparable reason (%s) verbatim', (reason, expected) => {
    expect(baselineNoteText({ campaign_id: 41, comparable: false, reason })).toBe(expected)
  })
})

describe('baselineChipText', () => {
  it('is null without a baseline (the chip is omitted)', () => {
    expect(baselineChipText(null)).toBeNull()
  })

  it('drops the 涨跌 prefix for the comparable chip (the label already says 涨跌基准)', () => {
    expect(baselineChipText({ campaign_id: 41, comparable: true })).toBe('较批次 #41')
  })

  it('shares the incomparable branches with the page note', () => {
    const baseline = { campaign_id: 41, comparable: false, reason: 'suite_changed' }
    expect(baselineChipText(baseline)).toBe(baselineNoteText(baseline))
  })
})

// Coverage-gate watermark (ticket 92, spec 0014 decision A): N is the
// contract's missing_suites; M is missing + the row's non-null suite scores
// so the denominator always matches the scores visible in the row. The 「缺」
// is load-bearing against the ScoreCell watermark's judged-numerator caliber.
describe('incompleteWatermark', () => {
  function makeRow(overrides: Partial<ReportRow>): ReportRow {
    return {
      model_db_id: 1,
      model_id: 'm',
      family: 'fam',
      total_score: null,
      total_delta: null,
      suite_scores: {},
      cells: [],
      ...overrides,
    }
  }

  it('is empty for complete rows and for live rows (no complete key)', () => {
    expect(incompleteWatermark(makeRow({ complete: true, suite_scores: { a: 80 } }))).toBe('')
    expect(incompleteWatermark(makeRow({ suite_scores: { a: 80 } }))).toBe('')
  })

  it('derives M as missing + judged so it matches the visible scores', () => {
    // Five covered suites, two judged, one emptied post-judging (null, not
    // gating) — the denominator counts only the gating suites: 2 missing +
    // 2 judged = 4, and the emptied suite's null stays out of M.
    const row = makeRow({
      complete: false,
      missing_suites: 2,
      suite_scores: { reasoning: 80, coding: 70, math: null, chinese: null, ifeval: null },
    })
    expect(incompleteWatermark(row)).toBe('判分不完整,缺 2/4 维度')
  })

  it('treats absent suite keys as unjudged (only non-null values count)', () => {
    const row = makeRow({ complete: false, missing_suites: 3, suite_scores: { reasoning: 55 } })
    expect(incompleteWatermark(row)).toBe('判分不完整,缺 3/4 维度')
  })

  it('degrades to a zero numerator when missing_suites is absent', () => {
    const row = makeRow({ complete: false, suite_scores: { reasoning: 55 } })
    expect(incompleteWatermark(row)).toBe('判分不完整,缺 0/1 维度')
  })
})
