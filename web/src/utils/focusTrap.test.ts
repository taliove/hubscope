// Unit tests for the Tab-wrap decision of the self-built modal focus trap
// (GH #116 check MEDIUM-2). DOM-free by construction: the decision operates
// on identity and array position, so plain values stand in for elements.
// The load-bearing invariants: forward Tab wraps last→first, Shift+Tab
// wraps first→last, focus OUTSIDE the surface wraps to the boundary in the
// direction of travel, and mid-list Tab is left to the browser (null).
import { describe, it, expect } from 'vitest'
import { trapTabTarget } from '@/utils/focusTrap'

const F = ['close', 'retry', 'detail'] as const

describe('trapTabTarget', () => {
  it('forward Tab on the last element wraps to the first', () => {
    expect(trapTabTarget('detail', F, false)).toBe('close')
  })

  it('Shift+Tab on the first element wraps to the last', () => {
    expect(trapTabTarget('close', F, true)).toBe('detail')
  })

  it('mid-list Tab is left to the browser in both directions', () => {
    expect(trapTabTarget('retry', F, false)).toBeNull()
    expect(trapTabTarget('retry', F, true)).toBeNull()
    expect(trapTabTarget('close', F, false)).toBeNull()
    expect(trapTabTarget('detail', F, true)).toBeNull()
  })

  it('focus outside the surface wraps to the boundary in the direction of travel', () => {
    // E.g. the browser-chrome cycle re-entering the page, or a element that
    // just unmounted — forward lands on the first, backward on the last.
    expect(trapTabTarget('stray', F, false)).toBe('close')
    expect(trapTabTarget('stray', F, true)).toBe('detail')
    expect(trapTabTarget(null, F, false)).toBe('close')
    expect(trapTabTarget(null, F, true)).toBe('detail')
  })

  it('a single focusable element traps on itself in both directions', () => {
    expect(trapTabTarget('only', ['only'], false)).toBe('only')
    expect(trapTabTarget('only', ['only'], true)).toBe('only')
  })

  it('an empty focusable list never hijacks (the wrapper swallows Tab itself)', () => {
    expect(trapTabTarget('anything', [], false)).toBeNull()
    expect(trapTabTarget(null, [], true)).toBeNull()
  })
})
