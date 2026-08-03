import { afterEach, describe, expect, it, vi } from 'vitest'

// chartMotion caches the matchMedia verdict lazily on first call, so each
// test re-imports the module fresh with its own stubbed environment.
async function freshModule() {
  vi.resetModules()
  return import('./chartMotion')
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('chartAnimationEnabled (spec 0018: reduced-motion zeroes JS motion)', () => {
  it('enables the entry animation when reduced motion is not requested', async () => {
    vi.stubGlobal('window', { matchMedia: () => ({ matches: false }) })
    const m = await freshModule()
    expect(m.chartAnimationEnabled()).toBe(true)
  })

  it('zeroes the entry animation under prefers-reduced-motion', async () => {
    vi.stubGlobal('window', { matchMedia: () => ({ matches: true }) })
    const m = await freshModule()
    expect(m.prefersReducedMotion()).toBe(true)
    expect(m.chartAnimationEnabled()).toBe(false)
  })

  it('defaults to enabled when no window/matchMedia exists (node, SSR)', async () => {
    const m = await freshModule()
    expect(m.chartAnimationEnabled()).toBe(true)
  })

  it('keeps the entry duration inside the spec 800–1200ms band', async () => {
    const m = await freshModule()
    expect(m.CHART_ANIMATION_DURATION_MS).toBeGreaterThanOrEqual(800)
    expect(m.CHART_ANIMATION_DURATION_MS).toBeLessThanOrEqual(1200)
  })
})
