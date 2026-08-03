import { describe, expect, it } from 'vitest'
import { parseBatchQuery, resolveInitialBatchId } from './batchSelect'
import type { Campaign } from '@/api/types'

type BatchStub = Pick<Campaign, 'id' | 'status'>

function batch(id: number, status: Campaign['status']): BatchStub {
  return { id, status }
}

describe('parseBatchQuery', () => {
  it('parses a positive integer batch id', () => {
    expect(parseBatchQuery('12')).toBe(12)
  })

  it('takes the first value of a repeated query param', () => {
    expect(parseBatchQuery(['7', '9'])).toBe(7)
  })

  it('rejects missing or empty values', () => {
    expect(parseBatchQuery(undefined)).toBeNull()
    expect(parseBatchQuery(null)).toBeNull()
    expect(parseBatchQuery('')).toBeNull()
    expect(parseBatchQuery([])).toBeNull()
  })

  it('rejects non-numeric, fractional and non-positive values', () => {
    expect(parseBatchQuery('abc')).toBeNull()
    expect(parseBatchQuery('1.5')).toBeNull()
    expect(parseBatchQuery('0')).toBeNull()
    expect(parseBatchQuery('-3')).toBeNull()
  })
})

describe('resolveInitialBatchId', () => {
  const list: BatchStub[] = [batch(5, 'running'), batch(4, 'done'), batch(3, 'done')]

  it('locates the query-specified batch when it exists (header progress entry)', () => {
    expect(resolveInitialBatchId(list, 5)).toBe(5)
  })

  it('leads with the newest batch overall when no query is given (2026-08-03 ruling)', () => {
    expect(resolveInitialBatchId(list, null)).toBe(5)
  })

  it('leads with the newest batch overall when the query batch is gone', () => {
    expect(resolveInitialBatchId(list, 99)).toBe(5)
  })

  it('falls back to the newest batch of any state when none is done', () => {
    expect(resolveInitialBatchId([batch(2, 'running'), batch(1, 'failed')], null)).toBe(2)
  })

  it('returns null for an empty campaign list', () => {
    expect(resolveInitialBatchId([], 5)).toBeNull()
    expect(resolveInitialBatchId([], null)).toBeNull()
  })
})
