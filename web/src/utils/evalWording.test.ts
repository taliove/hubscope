// Unit tests for the shared eval-board wording (ticket 76 check follow-up).
// These strings have multiple consumers (page alerts, the toolbar note and
// the EvalCard), so the verbatim copy is pinned here against drift.
import { describe, it, expect } from 'vitest'
import { failedBatchWarning, baselineNoteText, baselineChipText } from '@/utils/evalWording'

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
