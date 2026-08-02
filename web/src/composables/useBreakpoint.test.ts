// Unit tests for the shared 1024px breakpoint subscription (GH #94, check
// MEDIUM): initial state seeding, change propagation, single shared
// MediaQueryList across subscribers, and teardown on last release. The
// project has no DOM test environment (all tests run under node), so the
// tests stub the tiny slice of `window.matchMedia` the module touches —
// same precedent as visibilityPoll.test.ts stubbing `document`.
import { describe, it, expect, vi, afterEach } from 'vitest'
import { isReadonly } from 'vue'
import { subscribeBreakpoint } from '@/composables/useBreakpoint'

type Listener = () => void

// Minimal MediaQueryList stub: a mutable `matches` flag plus an event target
// just rich enough for change listeners.
function stubMatchMedia(initialMatches: boolean) {
  const listeners = new Set<Listener>()
  const mql = {
    matches: initialMatches,
    addEventListener: (_type: string, listener: Listener) => {
      listeners.add(listener)
    },
    removeEventListener: (_type: string, listener: Listener) => {
      listeners.delete(listener)
    },
    fireChange: (matches: boolean) => {
      mql.matches = matches
      for (const listener of [...listeners]) listener()
    },
    listenerCount: () => listeners.size,
  }
  const matchMedia = vi.fn(() => mql)
  vi.stubGlobal('window', { matchMedia })
  return { mql, matchMedia }
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('subscribeBreakpoint', () => {
  it('seeds the initial state from the first subscriber', () => {
    stubMatchMedia(true)
    const sub = subscribeBreakpoint()
    expect(sub.isNarrow.value).toBe(true)
    sub.release()
  })

  it('updates the shared state when the query changes', () => {
    const { mql } = stubMatchMedia(false)
    const sub = subscribeBreakpoint()
    expect(sub.isNarrow.value).toBe(false)
    mql.fireChange(true)
    expect(sub.isNarrow.value).toBe(true)
    mql.fireChange(false)
    expect(sub.isNarrow.value).toBe(false)
    sub.release()
  })

  it('shares one MediaQueryList across subscribers', () => {
    const { mql, matchMedia } = stubMatchMedia(true)
    const a = subscribeBreakpoint()
    const b = subscribeBreakpoint()
    expect(matchMedia).toHaveBeenCalledTimes(1)
    expect(matchMedia).toHaveBeenCalledWith('(max-width: 1023px)')
    expect(mql.listenerCount()).toBe(1)
    // One change reaches both subscribers (same shared ref).
    mql.fireChange(false)
    expect(a.isNarrow.value).toBe(false)
    expect(b.isNarrow.value).toBe(false)
    // Releasing the first subscriber keeps the shared query alive.
    a.release()
    expect(mql.listenerCount()).toBe(1)
    b.release()
  })

  it('returns a readonly ref so consumers cannot mutate shared state', () => {
    stubMatchMedia(false)
    const sub = subscribeBreakpoint()
    expect(isReadonly(sub.isNarrow)).toBe(true)
    sub.release()
  })

  it('tears down on last release and re-creates on the next subscription', () => {
    const first = stubMatchMedia(false)
    const sub = subscribeBreakpoint()
    sub.release()
    expect(first.mql.listenerCount()).toBe(0)

    // A fresh subscription after full teardown re-creates the query.
    const second = stubMatchMedia(true)
    const again = subscribeBreakpoint()
    expect(second.matchMedia).toHaveBeenCalledTimes(1)
    expect(again.isNarrow.value).toBe(true)
    again.release()
  })
})
