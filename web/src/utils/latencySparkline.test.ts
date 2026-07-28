// Unit tests for the latency sparkline geometry (EndpointCard sparkline).
// The x-slot formula is the load-bearing piece: it must reproduce the dots
// strip's flex+gap layout exactly, or the curve drifts off the dots above.
import { describe, it, expect } from 'vitest'
import {
  LATENCY_DEGRADE_FACTOR,
  MIN_Y_RANGE_MS,
  SPARKLINE_GAP_PX,
  bucketCenterX,
  bucketSlotWidth,
  buildLatencyAreaPath,
  buildLatencySegments,
  latencyThresholdMs,
  latencyY,
  sparklineYMax,
  thresholdVisible,
  thresholdY,
} from '@/utils/latencySparkline'

// A strip that divides exactly: 24 slots of 10px + 23 gaps of 2px.
const EXACT_WIDTH = 24 * 10 + 23 * SPARKLINE_GAP_PX // 286

describe('bucket slot geometry', () => {
  it('derives the slot width from the strip width and the shared gap', () => {
    expect(bucketSlotWidth(EXACT_WIDTH)).toBe(10)
  })

  it('centers bucket 0 in its slot', () => {
    expect(bucketCenterX(0, EXACT_WIDTH)).toBe(5)
  })

  it('advances by slot + gap per bucket', () => {
    expect(bucketCenterX(1, EXACT_WIDTH)).toBe(17)
    expect(bucketCenterX(23, EXACT_WIDTH)).toBe(281)
  })

  it('keeps the last bucket center inside the strip for arbitrary widths', () => {
    const width = 237 // odd card width — flex still distributes it
    const center = bucketCenterX(23, width)
    expect(center).toBeGreaterThan(0)
    expect(center).toBeLessThan(width)
    // The last slot's right edge lands exactly on the strip edge.
    expect(center + bucketSlotWidth(width) / 2).toBeCloseTo(width, 6)
  })
})

describe('sparklineYMax', () => {
  it('is peak-driven with 25% headroom above the floor', () => {
    expect(sparklineYMax([null, 1000, 2000, null])).toBe(2500)
  })

  it('floors at MIN_Y_RANGE_MS so sub-second jitter never inflates', () => {
    expect(MIN_Y_RANGE_MS).toBe(1000)
    expect(sparklineYMax([100, 200])).toBe(1000)
    expect(sparklineYMax([799])).toBe(1000) // 799 x 1.25 = 998.75 < floor
  })

  it('leaves the floor exactly at the boundary peak', () => {
    expect(sparklineYMax([800])).toBe(1000) // 800 x 1.25 = 1000
  })

  it('falls back to the floor when there is no data', () => {
    expect(sparklineYMax([null, null])).toBe(1000)
    expect(sparklineYMax([])).toBe(1000)
  })
})

describe('latencyY', () => {
  it('maps 0 to the bottom and the scale max to the top headroom', () => {
    expect(latencyY(0, 28, 110)).toBe(28)
    expect(latencyY(110, 28, 110)).toBe(0)
    expect(latencyY(100, 28, 110)).toBeCloseTo(28 - (100 / 110) * 28, 6)
  })

  it('never divides by zero on an empty scale', () => {
    expect(latencyY(5, 28, 0)).toBe(28)
  })
})

describe('threshold line', () => {
  it('matches the status machine degradation factor of 2', () => {
    expect(LATENCY_DEGRADE_FACTOR).toBe(2)
    expect(latencyThresholdMs(500)).toBe(1000)
  })

  it('draws the threshold at 2x the baseline, never 1x', () => {
    // baseline 500 -> threshold 1000; peak 2000 -> yMax 2500. The line sits
    // at latencyY(1000), not latencyY(500).
    const yMax = sparklineYMax([100, 2000])
    const y = thresholdY(500, 28, yMax)
    expect(y).toBeCloseTo(latencyY(1000, 28, yMax), 6)
    expect(y).not.toBeCloseTo(latencyY(500, 28, yMax), 6)
    expect(y).toBeCloseTo(28 * 0.6, 6)
  })

  it('appears when the 2x threshold fits the range (peak >= 0.8x threshold)', () => {
    // baseline 500 -> threshold 1000; peak 800 -> yMax 1000: boundary, shown.
    expect(thresholdVisible(500, sparklineYMax([800]))).toBe(true)
    // Comfortably above the threshold.
    expect(thresholdVisible(500, sparklineYMax([2000]))).toBe(true)
  })

  it('stays hidden when the threshold falls outside the range', () => {
    // baseline 600 -> threshold 1200; peak 700 -> yMax = max(875, 1000
    // floor) = 1000 < 1200: hidden, the floor alone never reveals it.
    expect(thresholdVisible(600, sparklineYMax([700]))).toBe(false)
    expect(thresholdVisible(600, sparklineYMax([100]))).toBe(false)
    // A threshold exactly at the floor boundary does show (T <= yMax).
    expect(thresholdVisible(500, sparklineYMax([700]))).toBe(true)
  })

  it('stays hidden without a baseline, regardless of the range', () => {
    expect(thresholdVisible(null, sparklineYMax([99999]))).toBe(false)
  })

  it('aligns a bucket at exactly 2x baseline with the threshold line', () => {
    const values: (number | null)[] = Array(24).fill(null)
    values[7] = 1000 // exactly 2x the 500 baseline
    const yMax = sparklineYMax(values)
    expect(thresholdVisible(500, yMax)).toBe(true)
    const segments = buildLatencySegments(values, EXACT_WIDTH, 28, yMax)
    expect(segments[0].points[0].y).toBeCloseTo(thresholdY(500, 28, yMax), 6)
  })
})

describe('buildLatencySegments', () => {
  const HEIGHT = 28

  it('returns nothing when every bucket is null (placeholder state)', () => {
    expect(buildLatencySegments([null, null, null], EXACT_WIDTH, HEIGHT, 0)).toEqual([])
  })

  it('breaks the line at null buckets into separate segments', () => {
    const values: (number | null)[] = Array(24).fill(null)
    values[2] = 100
    values[3] = 200
    values[10] = 300
    values[11] = 400
    const segments = buildLatencySegments(values, EXACT_WIDTH, HEIGHT, 440)
    expect(segments).toHaveLength(2)
    expect(segments[0].isolated).toBe(false)
    expect(segments[1].isolated).toBe(false)
    expect(segments[0].points.map(p => p.x)).toEqual([bucketCenterX(2, EXACT_WIDTH), bucketCenterX(3, EXACT_WIDTH)])
  })

  it('flags a lone bucket with null neighbors as isolated, without a fill', () => {
    const values: (number | null)[] = Array(24).fill(null)
    values[5] = 250
    const segments = buildLatencySegments(values, EXACT_WIDTH, HEIGHT, 275)
    expect(segments).toHaveLength(1)
    expect(segments[0].isolated).toBe(true)
    expect(segments[0].points).toHaveLength(1)
    expect(segments[0].areaPath).toBeNull()
  })

  it('flags a lone bucket at the window edge as isolated', () => {
    const values: (number | null)[] = Array(24).fill(null)
    values[0] = 250
    const segments = buildLatencySegments(values, EXACT_WIDTH, HEIGHT, 275)
    expect(segments[0].isolated).toBe(true)
  })

  it('does not flag adjacent pairs at the window edge as isolated', () => {
    const values: (number | null)[] = Array(24).fill(null)
    values[22] = 250
    values[23] = 260
    const segments = buildLatencySegments(values, EXACT_WIDTH, HEIGHT, 286)
    expect(segments).toHaveLength(1)
    expect(segments[0].isolated).toBe(false)
    expect(segments[0].areaPath).not.toBeNull()
  })
})

describe('buildLatencyAreaPath', () => {
  const HEIGHT = 28

  it('closes along the strip bottom: line, drop at the last point, back to the first', () => {
    const points = [
      { x: 5, y: 10 },
      { x: 17, y: 6 },
      { x: 29, y: 12 },
    ]
    expect(buildLatencyAreaPath(points, HEIGHT)).toBe('M 5.00,10.00 L 17.00,6.00 L 29.00,12.00 L 29.00,28 L 5.00,28 Z')
  })

  it('breaks the fill at null buckets together with the line', () => {
    const values: (number | null)[] = Array(24).fill(null)
    values[2] = 100
    values[3] = 200
    values[10] = 300
    values[11] = 400
    const segments = buildLatencySegments(values, EXACT_WIDTH, HEIGHT, 500)
    expect(segments).toHaveLength(2)
    for (const segment of segments) {
      expect(segment.areaPath).not.toBeNull()
      expect(segment.areaPath).toMatch(/^M /)
      expect(segment.areaPath).toMatch(/ Z$/)
    }
    // Two independent closed shapes, not one bridged fill.
    expect(segments[0].areaPath).not.toBe(segments[1].areaPath)
    expect(segments[0].areaPath).toContain(`L ${bucketCenterX(3, EXACT_WIDTH).toFixed(2)},${HEIGHT}`)
    expect(segments[1].areaPath).toContain(`L ${bucketCenterX(11, EXACT_WIDTH).toFixed(2)},${HEIGHT}`)
  })
})
