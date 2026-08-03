import { describe, it, expect } from 'vitest'
import { tweenNumber, easeOutCubic, TWEEN_DURATION_MS, type TweenDrivers } from '@/utils/numberTween'

// Fake frame clock: raf callbacks queue up and run when advance() moves the
// clock — no real timers involved.
function fakeDrivers(startAt = 0) {
  let now = startAt
  let queue: ((t: number) => void)[] = []
  const drivers: TweenDrivers = {
    now: () => now,
    raf: cb => {
      queue.push(cb)
      return queue.length
    },
    cancelRaf: () => {},
  }
  return {
    drivers,
    advance(ms: number) {
      now += ms
      const q = queue
      queue = []
      for (const cb of q) cb(now)
    },
  }
}

describe('easeOutCubic', () => {
  it('anchors both ends and stays monotonic', () => {
    expect(easeOutCubic(0)).toBe(0)
    expect(easeOutCubic(1)).toBe(1)
    let prev = 0
    for (let t = 0.05; t <= 1; t += 0.05) {
      const v = easeOutCubic(t)
      expect(v).toBeGreaterThan(prev)
      prev = v
    }
  })
})

describe('tweenNumber', () => {
  it('tweens from → to over the duration and lands exactly on the terminal value', () => {
    const { drivers, advance } = fakeDrivers()
    const seen: number[] = []
    tweenNumber(0, 100, v => seen.push(v), { durationMs: 600, reducedMotion: false, drivers })
    advance(300) // halfway: eased past linear midpoint
    const mid = seen.at(-1)!
    expect(mid).toBeGreaterThan(50)
    expect(mid).toBeLessThan(100)
    advance(300)
    expect(seen.at(-1)).toBe(100)
    advance(600) // no further emissions after settling
    expect(seen.at(-1)).toBe(100)
    expect(seen.filter(v => v === 100)).toHaveLength(1)
  })

  it('reduced motion renders the terminal value immediately and never schedules frames', () => {
    const { drivers, advance } = fakeDrivers()
    const seen: number[] = []
    tweenNumber(0, 100, v => seen.push(v), { reducedMotion: true, drivers })
    expect(seen).toEqual([100])
    advance(1000)
    expect(seen).toEqual([100])
  })

  it('no-op tween (from === to) settles immediately', () => {
    const { drivers } = fakeDrivers()
    const seen: number[] = []
    tweenNumber(42, 42, v => seen.push(v), { reducedMotion: false, drivers })
    expect(seen).toEqual([42])
  })

  it('cancel stops further emissions', () => {
    const { drivers, advance } = fakeDrivers()
    const seen: number[] = []
    const cancel = tweenNumber(0, 100, v => seen.push(v), { durationMs: 600, reducedMotion: false, drivers })
    advance(300)
    const count = seen.length
    cancel()
    advance(300)
    expect(seen.length).toBe(count)
  })

  it('default duration sits inside the spec band (500–800ms)', () => {
    expect(TWEEN_DURATION_MS).toBeGreaterThanOrEqual(500)
    expect(TWEEN_DURATION_MS).toBeLessThanOrEqual(800)
  })
})
