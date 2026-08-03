import { describe, expect, it } from 'vitest'
import { sparklineBarLayout, topRoundedBarPath } from './sparklineBars'

describe('sparklineBarLayout — empty-state discipline', () => {
  it('returns null for an empty series (host renders the placeholder track)', () => {
    expect(sparklineBarLayout([], 120, 32, 2)).toBeNull()
  })

  it('returns null for an all-zero series (zero reading never reads as data)', () => {
    expect(sparklineBarLayout([0, 0, 0, 0], 120, 32, 2)).toBeNull()
  })

  it('returns null for an all-null series', () => {
    expect(sparklineBarLayout([null, null], 120, 32, 2)).toBeNull()
  })
})

describe('sparklineBarLayout — normalization', () => {
  it('normalizes the max bucket to the full track height (upper bound)', () => {
    const bars = sparklineBarLayout([3, 10, 4], 120, 32, 2)
    expect(bars).not.toBeNull()
    const peak = bars!.find(b => b.height === 32)
    expect(peak).toBeDefined()
    // Other buckets scale proportionally and never exceed the track.
    expect(bars![0].height).toBeCloseTo((3 / 10) * 32, 9)
    expect(bars![2].height).toBeCloseTo((4 / 10) * 32, 9)
    for (const b of bars!) {
      expect(b.height).toBeGreaterThan(0)
      expect(b.height).toBeLessThanOrEqual(32)
      expect(b.y).toBeCloseTo(32 - b.height, 9)
    }
  })

  it('renders a single-peak series as one full-height bar', () => {
    const bars = sparklineBarLayout([0, 0, 42, 0], 120, 32, 2)
    expect(bars).toHaveLength(1)
    expect(bars![0].height).toBe(32)
    expect(bars![0].y).toBe(0)
  })

  it('keeps equal-width bars with the fixed 2px gap across the full width', () => {
    const bars = sparklineBarLayout([1, 2, 3, 4], 120, 32, 2)
    expect(bars).toHaveLength(4)
    const slot = (120 - 3 * 2) / 4
    bars!.forEach((b, i) => {
      expect(b.width).toBeCloseTo(slot, 9)
      expect(b.x).toBeCloseTo(i * (slot + 2), 9)
    })
    // The last bar lands flush on the right edge.
    const last = bars![3]
    expect(last.x + last.width).toBeCloseTo(120, 9)
  })

  it('keeps null buckets as empty slots (no bar, no bridging)', () => {
    const bars = sparklineBarLayout([5, null, 5], 120, 32, 2)
    expect(bars).toHaveLength(2)
    // The middle slot stays empty; the third bar sits at its own position.
    const slot = (120 - 2 * 2) / 3
    expect(bars![1].x).toBeCloseTo(2 * (slot + 2), 9)
  })

  it('returns null when the gap leaves no room for bars', () => {
    expect(sparklineBarLayout([1, 2, 3, 4, 5], 4, 32, 2)).toBeNull()
  })
})

describe('topRoundedBarPath', () => {
  it('rounds the top corners only (bottom edge stays flush on the track)', () => {
    const d = topRoundedBarPath(10, 8, 20, 24, 2)
    // Starts at the bottom-left corner and closes along the bottom edge.
    expect(d.startsWith('M 10 32')).toBe(true)
    expect(d.endsWith('V 32 Z')).toBe(true)
    // Two top-corner arcs, no arc near the bottom.
    expect(d.match(/Q /g)).toHaveLength(2)
  })

  it('clamps the radius to short bars (never self-intersects)', () => {
    // radius 2, bar height 1 → effective radius 1.
    const d = topRoundedBarPath(0, 31, 10, 1, 2)
    expect(d).toContain('Q 0 31 1 31')
  })

  it('clamps the radius to narrow bars', () => {
    const d = topRoundedBarPath(0, 0, 3, 32, 2)
    expect(d).toContain('Q 0 0 1.5 0')
  })

  it('degenerates to a plain rectangle when the radius clamps to zero', () => {
    expect(topRoundedBarPath(0, 31, 10, 0, 2)).toBe('M 0 31 H 10 V 31 H 0 Z')
  })
})
