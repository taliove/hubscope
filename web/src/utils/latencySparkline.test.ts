// Unit tests for the latency sparkline geometry (EndpointCard sparkline).
// The x-slot formula is the load-bearing piece: it must reproduce the dots
// strip's flex+gap layout exactly, or the curve drifts off the dots above.
import { describe, it, expect } from 'vitest'
import {
  LATENCY_DEGRADE_FACTOR,
  SPARKLINE_GAP_PX,
  bucketCenterX,
  bucketSlotWidth,
  buildLatencySegments,
  latencyThresholdMs,
  latencyY,
  sparklineYMax,
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
  it('is the peak bucket value with 10% headroom when no baseline', () => {
    expect(sparklineYMax([null, 100, 200, null], null)).toBeCloseTo(220, 6)
  })

  it('uses the 2x baseline threshold when it exceeds the peak', () => {
    // baseline 500 -> threshold 1000 -> yMax 1100
    expect(sparklineYMax([100, 200], 500)).toBeCloseTo(1100, 6)
  })

  it('keeps the peak when it exceeds the 2x baseline threshold', () => {
    // baseline 500 -> threshold 1000 < peak 2000 -> yMax 2200
    expect(sparklineYMax([100, 2000], 500)).toBeCloseTo(2200, 6)
  })

  it('is zero when there is no data and no baseline', () => {
    expect(sparklineYMax([null, null], null)).toBe(0)
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
    // baseline 500, peak 200 -> yMax 1100; the line sits at latencyY(1000),
    // not latencyY(500).
    const yMax = sparklineYMax([100, 200], 500)
    const y = thresholdY(500, 28, yMax)
    expect(y).toBeCloseTo(latencyY(1000, 28, yMax), 6)
    expect(y).not.toBeCloseTo(latencyY(500, 28, yMax), 6)
    expect(y).toBeCloseTo(28 / 11, 6)
  })

  it('aligns a bucket at exactly 2x baseline with the threshold line', () => {
    const values: (number | null)[] = Array(24).fill(null)
    values[7] = 1000 // exactly 2x the 500 baseline
    const yMax = sparklineYMax(values, 500)
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

  it('flags a lone bucket with null neighbors as isolated', () => {
    const values: (number | null)[] = Array(24).fill(null)
    values[5] = 250
    const segments = buildLatencySegments(values, EXACT_WIDTH, HEIGHT, 275)
    expect(segments).toHaveLength(1)
    expect(segments[0].isolated).toBe(true)
    expect(segments[0].points).toHaveLength(1)
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
  })
})
