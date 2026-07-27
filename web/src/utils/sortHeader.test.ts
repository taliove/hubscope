// Unit tests for the column-header sort state machine (ticket 78,
// descending-only ruling): click to rank by a column, click the active
// column to fall back to the total, click another column to switch.
import { describe, it, expect } from 'vitest'
import { nextSortKey } from '@/utils/sortHeader'

describe('nextSortKey', () => {
  it('ranks by the clicked column descending', () => {
    expect(nextSortKey('total', 'cap_reasoning')).toBe('cap_reasoning')
  })

  it('falls back to the total when the active column is clicked again', () => {
    expect(nextSortKey('cap_reasoning', 'cap_reasoning')).toBe('total')
  })

  it('switches columns directly without passing through the total', () => {
    expect(nextSortKey('cap_reasoning', 'cap_coding')).toBe('cap_coding')
  })

  it('keeps the total when the total column is clicked while active', () => {
    expect(nextSortKey('total', 'total')).toBe('total')
  })
})
