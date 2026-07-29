// Unit tests for the GH #42 cost pure functions: wall-clock derivation and
// the batch summary line shared by the progress grid and the report page.
import { describe, it, expect } from 'vitest'
import { batchCostSummary, wallClockMs } from '@/utils/evalCost'

describe('wallClockMs', () => {
  it('uses finished_at on a settled batch', () => {
    expect(wallClockMs('2026-07-29T01:00:00Z', '2026-07-29T01:03:12Z', 0)).toBe(192_000)
  })

  it('uses now while the batch is in flight', () => {
    const now = new Date('2026-07-29T01:05:00Z').getTime()
    expect(wallClockMs('2026-07-29T01:00:00Z', null, now)).toBe(300_000)
  })

  it('is null without a start and never negative', () => {
    expect(wallClockMs(null, null, 0)).toBeNull()
    expect(wallClockMs('not-a-date', null, 0)).toBeNull()
    expect(wallClockMs('2026-07-29T01:00:00Z', '2026-07-29T00:59:00Z', 0)).toBe(0)
  })
})

describe('batchCostSummary', () => {
  it('lays judging time and wall-clock side by side with the token split', () => {
    const line = batchCostSummary(
      { latency_ms: 12_300, input_tokens: 10_000, output_tokens: 2_300 },
      '2026-07-29T01:00:00Z',
      '2026-07-29T01:03:12Z',
      0,
    )
    expect(line).toBe('判分耗时 12.3s · 批次用时 3 分 12 秒 · Token 12.3k(输入 10.0k / 输出 2.3k)')
  })

  it('renders the in-flight wall-clock from now and a dash when unknown', () => {
    const now = new Date('2026-07-29T01:05:00Z').getTime()
    expect(batchCostSummary({ latency_ms: 0, input_tokens: 0, output_tokens: 0 }, '2026-07-29T01:00:00Z', null, now)).toBe(
      '判分耗时 0.0s · 批次用时 5 分 0 秒 · Token 0(输入 0 / 输出 0)',
    )
    expect(batchCostSummary({ latency_ms: 500, input_tokens: 3, output_tokens: 4 }, null, null, now)).toBe(
      '判分耗时 0.5s · 批次用时 - · Token 7(输入 3 / 输出 4)',
    )
  })
})
