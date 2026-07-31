import { describe, it, expect } from 'vitest'
import { cardFilename } from '../cardImage'

describe('cardFilename', () => {
  const testDate = new Date('2026-07-31T15:30:00')

  it('generates full variant filename without scope', () => {
    expect(cardFilename(testDate, 'status')).toBe('hubscope-status-20260731-1530.png')
  })

  it('generates full variant filename with scope', () => {
    expect(cardFilename(testDate, 'status', 'group-abc')).toBe('hubscope-status-group-abc-20260731-1530.png')
  })

  it('generates compact variant filename without scope', () => {
    expect(cardFilename(testDate, 'status', undefined, 'compact')).toBe('hubscope-status-compact-20260731-1530.png')
  })

  it('generates compact variant filename with scope', () => {
    expect(cardFilename(testDate, 'status', 'group-xyz', 'compact')).toBe('hubscope-status-group-xyz-compact-20260731-1530.png')
  })

  it('preserves existing behavior when variant is explicitly full', () => {
    expect(cardFilename(testDate, 'status', 'test', 'full')).toBe('hubscope-status-test-20260731-1530.png')
  })

  it('works with eval card kind', () => {
    expect(cardFilename(testDate, 'eval', '批次15', 'compact')).toBe('hubscope-eval-批次15-compact-20260731-1530.png')
  })
})
