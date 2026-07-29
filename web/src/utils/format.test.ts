// Unit tests for the shared formatting helpers (GH #57 introduced
// formatTimeMinute; earlier helpers gained coverage here as the file's
// first test module).
import { describe, it, expect } from 'vitest'
import { formatTimeMinute } from '@/utils/format'

describe('formatTimeMinute', () => {
  it('drops the seconds (minute precision)', () => {
    // Local-time ISO without Z keeps the assertion timezone-independent.
    expect(formatTimeMinute('2026-07-29T14:35:42')).toBe('2026-07-29 14:35')
  })

  it('pads single-digit fields', () => {
    expect(formatTimeMinute('2026-01-05T03:07:59')).toBe('2026-01-05 03:07')
  })

  it('renders a dash for null and undefined', () => {
    expect(formatTimeMinute(null)).toBe('-')
    expect(formatTimeMinute(undefined)).toBe('-')
    expect(formatTimeMinute('')).toBe('-')
  })

  it('falls back to the raw value on an unparseable input', () => {
    expect(formatTimeMinute('not-a-time')).toBe('not-a-time')
  })
})
