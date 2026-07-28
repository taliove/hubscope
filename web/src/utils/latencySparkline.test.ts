// Unit tests for the latency sparkline geometry (EndpointCard sparkline).
// The x-slot formula is the load-bearing piece: it must reproduce the dots
// strip's flex+gap layout exactly, or the curve drifts off the dots above.
import { describe, it, expect } from 'vitest'
import {
  SPARKLINE_GAP_PX,
  bucketCenterX,
  bucketSlotWidth,
  buildLatencySegments,
  latencyY,
  sparklineYMax,
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

  it('uses the baseline when it exceeds the peak', () => {
    expect(sparklineYMax([100, 200], 500)).toBe(550)
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
