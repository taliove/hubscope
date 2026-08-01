import { describe, expect, it } from 'vitest'
import {
  availabilityBarWidth,
  entryLatencySeries,
  familyInitials,
  familyOptions,
  rowSparklineTone,
} from '@/utils/modelList'
import type { OverviewDot, OverviewEntry } from '@/api/types'

const entry = (over: Partial<OverviewEntry>): OverviewEntry =>
  ({
    endpoint_id: 1,
    model_id: 'm',
    protocol: 'openai',
    family: 'gpt',
    capability: 'chat',
    status: 'healthy',
    enabled: true,
    success_rate_24h: 0.99,
    p50_ms: 100,
    p95_ms: 200,
    dots_24h: [],
    baseline_p50_ms: null,
    ...over,
  }) as OverviewEntry

describe('familyInitials', () => {
  it('takes the first two letters of a single long Latin word', () => {
    expect(familyInitials('anthropic')).toBe('AN')
    expect(familyInitials('google')).toBe('GO')
    expect(familyInitials('qwen')).toBe('QW')
  })
  it('keeps a short word whole (≤3 chars), uppercased', () => {
    expect(familyInitials('gpt')).toBe('GPT')
    expect(familyInitials('x')).toBe('X')
  })
  it('takes the first character of each of the first three words', () => {
    expect(familyInitials('Ali Cloud')).toBe('AC')
    expect(familyInitials('one two three four')).toBe('OTT')
  })
  it('keeps a short CJK name whole, else the first character', () => {
    expect(familyInitials('阿里')).toBe('阿里')
    expect(familyInitials('深度求索')).toBe('深')
  })
  it('falls back to an em dash for empty input', () => {
    expect(familyInitials('')).toBe('—')
    expect(familyInitials('   ')).toBe('—')
    expect(familyInitials('---')).toBe('—')
  })
})

describe('availabilityBarWidth', () => {
  it('maps the rate onto the constant 0–100 scale', () => {
    expect(availabilityBarWidth(1)).toBe(100)
    expect(availabilityBarWidth(0.955)).toBeCloseTo(95.5)
    expect(availabilityBarWidth(0)).toBe(0)
  })
  it('renders null (no data) as an empty track', () => {
    expect(availabilityBarWidth(null)).toBe(0)
  })
  it('clamps out-of-range input defensively', () => {
    expect(availabilityBarWidth(1.2)).toBe(100)
    expect(availabilityBarWidth(-0.5)).toBe(0)
  })
})

describe('entryLatencySeries', () => {
  it('maps hourly P50 buckets, preserving null breaks', () => {
    const dots = [
      { bucket_start: 'a', total: 4, failures: 0, p50_ms: 120 },
      { bucket_start: 'b', total: 2, failures: 2, p50_ms: null },
      { bucket_start: 'c', total: 1, failures: 0, p50_ms: 90 },
    ] as OverviewDot[]
    expect(entryLatencySeries(dots)).toEqual([120, null, 90])
  })
})

describe('rowSparklineTone', () => {
  it('colors by display state: stable green / degraded yellow / incident red', () => {
    expect(rowSparklineTone(entry({ status: 'healthy' }))).toBe('success')
    expect(rowSparklineTone(entry({ status: 'degraded' }))).toBe('warning')
    expect(rowSparklineTone(entry({ status: 'down' }))).toBe('danger')
    expect(rowSparklineTone(entry({ status: 'failing' }))).toBe('danger')
  })
  it('renders disabled endpoints neutral whatever the status says', () => {
    expect(rowSparklineTone(entry({ enabled: false, status: 'down' }))).toBe('neutral')
  })
  it('falls back to neutral for payloads outside the status union', () => {
    expect(rowSparklineTone(entry({ status: 'bogus' as never }))).toBe('neutral')
  })
})

describe('familyOptions', () => {
  it('dedupes and sorts family values by code unit', () => {
    const entries = [entry({ family: 'qwen' }), entry({ family: 'gpt' }), entry({ family: 'qwen' })]
    expect(familyOptions(entries)).toEqual(['gpt', 'qwen'])
  })
  it('returns an empty list for an empty board', () => {
    expect(familyOptions([])).toEqual([])
  })
})
