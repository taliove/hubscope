// Unit tests for the shared formatting helpers. Two suites:
// - formatDuration / formatTokens (GH #42, ui-guidelines §5 成本指标条 + §7):
//   band boundaries pinned here are the registered caliber — sub-minute
//   one-decimal seconds, sub-hour minutes+seconds, beyond hours+minutes;
//   tokens raw below 1000, then one-decimal k/M.
// - formatTimeMinute (GH #57): minute-precision timestamp for the /board
//   batch meta line.
// - formatClockMinute (polish 2026-07-29): bare HH:mm for freshness labels.
import { describe, it, expect } from 'vitest'
import { formatDuration, formatTokens, formatTimeMinute, formatClockMinute } from '@/utils/format'

describe('formatDuration', () => {
  it('renders sub-minute durations as one-decimal seconds', () => {
    expect(formatDuration(0)).toBe('0.0s')
    expect(formatDuration(12300)).toBe('12.3s')
    expect(formatDuration(59_999)).toBe('60.0s')
  })

  it('renders sub-hour durations as minutes and seconds', () => {
    expect(formatDuration(60_000)).toBe('1 分 0 秒')
    expect(formatDuration(192_000)).toBe('3 分 12 秒')
    expect(formatDuration(3_599_000)).toBe('59 分 59 秒')
  })

  it('renders hour-plus durations as hours and minutes', () => {
    expect(formatDuration(3_600_000)).toBe('1 小时 0 分')
    expect(formatDuration(3_900_000)).toBe('1 小时 5 分')
    expect(formatDuration(7_320_000)).toBe('2 小时 2 分')
  })

  it('renders a dash for null', () => {
    expect(formatDuration(null)).toBe('-')
  })
})

describe('formatTokens', () => {
  it('renders sub-1000 counts raw', () => {
    expect(formatTokens(0)).toBe('0')
    expect(formatTokens(999)).toBe('999')
  })

  it('abbreviates thousands with one decimal', () => {
    expect(formatTokens(1000)).toBe('1.0k')
    expect(formatTokens(12_300)).toBe('12.3k')
    expect(formatTokens(999_999)).toBe('1000.0k')
  })

  it('abbreviates millions with one decimal', () => {
    expect(formatTokens(1_000_000)).toBe('1.0M')
    expect(formatTokens(1_200_000)).toBe('1.2M')
  })

  it('renders a dash for null', () => {
    expect(formatTokens(null)).toBe('-')
  })
})

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

describe('formatClockMinute', () => {
  it('renders a bare HH:mm clock reading', () => {
    expect(formatClockMinute('2026-07-29T14:35:42')).toBe('14:35')
  })

  it('pads single-digit fields', () => {
    expect(formatClockMinute('2026-01-05T03:07:59')).toBe('03:07')
  })

  it('renders a dash for null and undefined', () => {
    expect(formatClockMinute(null)).toBe('-')
    expect(formatClockMinute(undefined)).toBe('-')
  })

  it('falls back to the raw value on an unparseable input', () => {
    expect(formatClockMinute('not-a-time')).toBe('not-a-time')
  })
})
