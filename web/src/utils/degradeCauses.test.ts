// Unit tests for the degraded-cause sub-label pure functions (spec 0013):
// the cause vocabulary and the badge suffix. The join separator, the '· '
// prefix and the availability-first order are the load-bearing caliber
// pinned here — they mirror the backend degrade_causes contract.
import { describe, it, expect } from 'vitest'
import { DEGRADE_CAUSE_LABELS, degradeCauseSuffix } from '@/utils/degradeCauses'

describe('DEGRADE_CAUSE_LABELS', () => {
  it('maps every backend cause to a Chinese wording', () => {
    expect(DEGRADE_CAUSE_LABELS.availability).toBe('可用性')
    expect(DEGRADE_CAUSE_LABELS.latency).toBe('延迟')
  })
})

describe('degradeCauseSuffix', () => {
  it('renders a single availability hit', () => {
    expect(degradeCauseSuffix(['availability'])).toBe('· 可用性')
  })

  it('renders a single latency hit', () => {
    expect(degradeCauseSuffix(['latency'])).toBe('· 延迟')
  })

  it('joins a double hit with " + ", availability first', () => {
    expect(degradeCauseSuffix(['availability', 'latency'])).toBe('· 可用性 + 延迟')
  })

  it('renders nothing without causes', () => {
    expect(degradeCauseSuffix([])).toBe('')
  })
})
