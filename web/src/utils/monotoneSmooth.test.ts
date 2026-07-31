import { describe, expect, it } from 'vitest'
import {
  SMOOTH_SAMPLES_PER_SEGMENT,
  expandCategories,
  nearestRealIndex,
  smoothSeries,
} from './monotoneSmooth'

// Test helper: the sampled values of one original segment [i, i+1].
function segmentSamples(out: (number | null)[], i: number, k = SMOOTH_SAMPLES_PER_SEGMENT) {
  return out.slice(i * k, (i + 1) * k + 1)
}

describe('smoothSeries — shape and identity', () => {
  it('returns [] for empty input', () => {
    expect(smoothSeries([])).toEqual([])
  })

  it('returns a copy for a single point (no segment to smooth)', () => {
    expect(smoothSeries([42])).toEqual([42])
    expect(smoothSeries([null])).toEqual([null])
  })

  it('is the identity mapping when samplesPerSegment = 1', () => {
    const data = [1, 5, null, 2, 2]
    expect(smoothSeries(data, 1)).toEqual(data)
  })

  it('expands to (n-1)*k + 1 slots', () => {
    expect(smoothSeries([1, 2, 3], 8)).toHaveLength(17)
    expect(smoothSeries([1, 2, 3], 4)).toHaveLength(9)
  })

  it('passes through every real point exactly (interpolation, not approximation)', () => {
    const data = [3, 10, 4, 9, 1]
    const out = smoothSeries(data)
    data.forEach((v, i) => {
      expect(out[i * SMOOTH_SAMPLES_PER_SEGMENT]).toBe(v)
    })
  })
})

describe('smoothSeries — never invents extrema', () => {
  it('keeps every sample within its segment endpoints on zigzag input', () => {
    const data = [0, 100, 5, 90, 10, 80]
    const out = smoothSeries(data)
    for (let i = 0; i < data.length - 1; i++) {
      const lo = Math.min(data[i], data[i + 1])
      const hi = Math.max(data[i], data[i + 1])
      for (const v of segmentSamples(out, i)) {
        expect(v).not.toBeNull()
        expect(v as number).toBeGreaterThanOrEqual(lo - 1e-9)
        expect(v as number).toBeLessThanOrEqual(hi + 1e-9)
      }
    }
  })

  it('global output min/max never exceed the data min/max', () => {
    const data = [50, 3, 97, 20, 20, 88, 1]
    const out = smoothSeries(data).filter((v): v is number => v !== null)
    expect(Math.min(...out)).toBeGreaterThanOrEqual(Math.min(...data) - 1e-9)
    expect(Math.max(...out)).toBeLessThanOrEqual(Math.max(...data) + 1e-9)
  })

  it('is monotone within every segment on monotone input', () => {
    const data = [1, 4, 9, 16, 25, 40]
    const out = smoothSeries(data)
    for (let i = 0; i < data.length - 1; i++) {
      const samples = segmentSamples(out, i) as number[]
      for (let s = 1; s < samples.length; s++) {
        expect(samples[s]).toBeGreaterThanOrEqual(samples[s - 1] - 1e-9)
      }
    }
  })

  it('flattens a plateau instead of overshooting it', () => {
    const data = [1, 5, 5, 5, 2]
    const out = smoothSeries(data)
    for (let i = 1; i <= 2; i++) {
      for (const v of segmentSamples(out, i)) expect(v).toBeCloseTo(5, 9)
    }
  })
})

describe('smoothSeries — null breaks (connectNulls:false discipline)', () => {
  it('keeps null slots null and breaks the curve around gaps', () => {
    const data = [1, 2, null, 4, 5]
    const k = SMOOTH_SAMPLES_PER_SEGMENT
    const out = smoothSeries(data)
    expect(out).toHaveLength(4 * k + 1)
    // Real points stay exact.
    expect(out[0]).toBe(1)
    expect(out[k]).toBe(2)
    expect(out[2 * k]).toBeNull()
    expect(out[3 * k]).toBe(4)
    expect(out[4 * k]).toBe(5)
    // The segment spanning the gap emits null interior slots — no invented
    // bridge across a measurement hole.
    for (let s = 1; s < k; s++) expect(out[k + s]).toBeNull()
    for (let s = 1; s < k; s++) expect(out[2 * k + s]).toBeNull()
    // Measured segments on both sides are smoothed normally.
    expect(out[1]).not.toBeNull()
    expect(out[3 * k + 1]).not.toBeNull()
  })

  it('handles an all-null series', () => {
    const out = smoothSeries([null, null, null])
    expect(out).toHaveLength(2 * SMOOTH_SAMPLES_PER_SEGMENT + 1)
    expect(out.every(v => v === null)).toBe(true)
  })

  it('handles a lone real point surrounded by nulls', () => {
    const k = SMOOTH_SAMPLES_PER_SEGMENT
    const out = smoothSeries([null, 7, null])
    expect(out[k]).toBe(7)
    expect(out.filter(v => v !== null)).toEqual([7])
  })

  it('handles leading and trailing nulls', () => {
    const k = SMOOTH_SAMPLES_PER_SEGMENT
    const out = smoothSeries([null, 1, 3, null])
    expect(out[0]).toBeNull()
    expect(out[k]).toBe(1)
    expect(out[2 * k]).toBe(3)
    expect(out[3 * k]).toBeNull()
    expect(out[k + 1]).not.toBeNull()
  })
})

describe('expandCategories', () => {
  it('places real labels at their original positions and blanks elsewhere', () => {
    const out = expandCategories(['10:00', '11:00', '12:00'], 4)
    expect(out).toHaveLength(9)
    expect(out[0]).toBe('10:00')
    expect(out[4]).toBe('11:00')
    expect(out[8]).toBe('12:00')
    expect(out[1]).toBe('')
    expect(out[7]).toBe('')
  })

  it('returns [] for empty input and identity for k = 1', () => {
    expect(expandCategories([], 4)).toEqual([])
    expect(expandCategories(['a', 'b'], 1)).toEqual(['a', 'b'])
  })
})

describe('nearestRealIndex', () => {
  it('snaps interpolated slots to the nearest real point', () => {
    // k = 8: slot 12 sits between real index 1 (slot 8) and 2 (slot 16).
    expect(nearestRealIndex(12, 5, 8)).toBe(2)
    expect(nearestRealIndex(11, 5, 8)).toBe(1)
    expect(nearestRealIndex(0, 5, 8)).toBe(0)
    expect(nearestRealIndex(32, 5, 8)).toBe(4)
  })

  it('clamps to the valid range', () => {
    expect(nearestRealIndex(999, 3, 8)).toBe(2)
  })
})
