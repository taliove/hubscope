// Unit tests for the alert-event kind mapping (GH #68, spec 0017 ticket 5).
// The vocabulary is frozen by spec 0017 — these tests pin every kind's
// Chinese label plus the defensive fallback (unknown kinds must never
// render as a bare English string nor claim a status color).
import { describe, it, expect } from 'vitest'
import { alertKindLabel, alertKindTagType } from '@/utils/alertKind'

describe('alertKindLabel', () => {
  it('keeps the five legacy kind labels unchanged', () => {
    expect(alertKindLabel('down')).toBe('故障')
    expect(alertKindLabel('recovered')).toBe('恢复')
    expect(alertKindLabel('score_drop')).toBe('分数大跌')
    expect(alertKindLabel('score_drop_skipped')).toBe('对比跳过')
    expect(alertKindLabel('test')).toBe('测试')
  })

  it('maps the spec 0017 kinds to the frozen words', () => {
    expect(alertKindLabel('group_down')).toBe('厂商组告警')
    expect(alertKindLabel('group_recovered')).toBe('厂商组恢复')
    expect(alertKindLabel('batch')).toBe('聚合发送')
    expect(alertKindLabel('quiet_summary')).toBe('静默摘要')
  })

  it('maps the spec 0018 T4 retirement kinds (GH #98)', () => {
    expect(alertKindLabel('retire_pending')).toBe('待退役')
    expect(alertKindLabel('retired')).toBe('已退役')
  })

  it('falls back to a placeholder for unknown kinds (never the raw string)', () => {
    expect(alertKindLabel('some_future_kind')).toBe('未知类型')
    expect(alertKindLabel('some_future_kind')).not.toBe('some_future_kind')
  })

  it('falls back to the placeholder for null/undefined', () => {
    expect(alertKindLabel(null)).toBe('未知类型')
    expect(alertKindLabel(undefined)).toBe('未知类型')
  })
})

describe('alertKindTagType', () => {
  it('maps fault kinds to danger and recovery kinds to success', () => {
    expect(alertKindTagType('down')).toBe('danger')
    expect(alertKindTagType('group_down')).toBe('danger')
    expect(alertKindTagType('recovered')).toBe('success')
    expect(alertKindTagType('group_recovered')).toBe('success')
  })

  it('maps score-drop kinds to warning (existing caliber)', () => {
    expect(alertKindTagType('score_drop')).toBe('warning')
    expect(alertKindTagType('score_drop_skipped')).toBe('warning')
  })

  it('maps delivery-form kinds to neutral info (not a health signal)', () => {
    expect(alertKindTagType('test')).toBe('info')
    expect(alertKindTagType('batch')).toBe('info')
    expect(alertKindTagType('quiet_summary')).toBe('info')
  })

  it('maps spec 0018 T4 retirement kinds (GH #98): retire_pending warning, retired info', () => {
    // retire_pending is warning (needs attention, like failures).
    expect(alertKindTagType('retire_pending')).toBe('warning')
    // retired is info (delivery record, not a health signal).
    expect(alertKindTagType('retired')).toBe('info')
  })

  it('falls back to info for unknown kinds (never claims a status color)', () => {
    expect(alertKindTagType('some_future_kind')).toBe('info')
  })

  it('falls back to info for null/undefined', () => {
    expect(alertKindTagType(null)).toBe('info')
    expect(alertKindTagType(undefined)).toBe('info')
  })
})
