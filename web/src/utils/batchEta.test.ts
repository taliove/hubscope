// Unit tests for the batch progress/ETA math (2026-08-03 live-board batch):
// judged-unit progress caliber, per-cell pace with campaign fallback, and
// pool-spread batch ETA.
import { describe, it, expect } from 'vitest'
import type { CampaignReport, ReportCell, ReportRow } from '@/api/types'
import { unitProgress, avgUnitMs, cellRemainingMs, rowRemainingMs, batchRemainingMs } from '@/utils/batchEta'

function cell(overrides: Partial<ReportCell> = {}): ReportCell {
  return {
    suite_key: 'mmlu',
    status: 'running',
    judged_cases: 5,
    expected_cases: 10,
    samples: 5,
    latency_ms: 10_000,
    ...overrides,
  }
}

function report(rows: Partial<ReportRow>[]): CampaignReport {
  return {
    id: 1,
    trigger: 'manual',
    status: 'running',
    started_at: '2026-08-03T00:00:00Z',
    finished_at: null,
    suites: [],
    rows: rows.map((r, i) => ({
      model_db_id: i + 1,
      model_id: `m${i + 1}`,
      family: 'f',
      total_score: null,
      total_delta: null,
      cells: [],
      ...r,
    })) as ReportRow[],
  } as unknown as CampaignReport
}

describe('unitProgress', () => {
  it('sums judged and expected cases across all cells', () => {
    const r = report([{ cells: [cell(), cell({ suite_key: 'gsm8k', judged_cases: 3, expected_cases: 12, status: 'pending' })] }])
    expect(unitProgress(r)).toEqual({ judged: 8, expected: 22 })
  })
})

describe('avgUnitMs', () => {
  it('averages latency over judged cases and ignores unjudged cells', () => {
    const r = report([
      { cells: [cell({ latency_ms: 10_000 }), cell({ suite_key: 'gsm8k', judged_cases: 0, latency_ms: 99_000 })] },
    ])
    expect(avgUnitMs(r)).toBe(2000)
  })

  it('is null when nothing is judged', () => {
    const r = report([{ cells: [cell({ judged_cases: 0 })] }])
    expect(avgUnitMs(r)).toBeNull()
  })
})

describe('cellRemainingMs', () => {
  it('estimates from the cell own pace', () => {
    // 5 remaining cases x 2000ms
    expect(cellRemainingMs(cell(), 999)).toBe(10_000)
  })

  it('falls back to the campaign pace for unjudged cells', () => {
    expect(cellRemainingMs(cell({ judged_cases: 0, latency_ms: 0 }), 500)).toBe(5000)
  })

  it('is null for terminal cells and fully covered cells', () => {
    expect(cellRemainingMs(cell({ status: 'done' }), 500)).toBeNull()
    expect(cellRemainingMs(cell({ judged_cases: 10 }), 500)).toBeNull()
  })

  it('is null when no pace is known', () => {
    expect(cellRemainingMs(cell({ judged_cases: 0, latency_ms: undefined }), null)).toBeNull()
  })
})

describe('rowRemainingMs', () => {
  it('sums remaining time across the model suites', () => {
    const row: Partial<ReportRow> = {
      cells: [cell(), cell({ suite_key: 'gsm8k', status: 'done', judged_cases: 10 })],
    }
    expect(rowRemainingMs(row as ReportRow, null)).toBe(10_000)
  })
})

describe('batchRemainingMs', () => {
  it('spreads remaining unit-seconds over the pool', () => {
    const r = report([
      { cells: [cell()] }, // 10s remaining
      { cells: [cell()] }, // 10s remaining
    ])
    expect(batchRemainingMs(r, 4)).toBe(5000)
  })

  it('is null with no pace and null with nothing remaining', () => {
    expect(batchRemainingMs(report([{ cells: [cell({ judged_cases: 0, latency_ms: undefined })] }]), 4)).toBeNull()
    expect(batchRemainingMs(report([{ cells: [cell({ status: 'done', judged_cases: 10 })] }]), 4)).toBeNull()
  })
})
