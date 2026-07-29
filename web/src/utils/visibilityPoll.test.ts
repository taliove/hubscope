// Unit tests for the visibility-aware poll wrapper (spec 0015 decision 5):
// hidden throttling/pause, immediate refresh on returning to the foreground,
// and clear() removing both the timer and the visibility listener. The
// project has no DOM test environment (all utils tests run under node), so
// the tests stub the tiny slice of `document` the wrapper touches.
import { describe, it, expect, vi, afterEach } from 'vitest'
import { createVisibilityPoll, type VisibilityPollHandle } from '@/utils/visibilityPoll'

type Listener = () => void

// Minimal document stub: visibility flag + an event-target just rich enough
// for visibilitychange listeners.
function stubDocument() {
  const listeners = new Set<Listener>()
  const doc = {
    hidden: false,
    addEventListener: (_type: string, listener: Listener) => {
      listeners.add(listener)
    },
    removeEventListener: (_type: string, listener: Listener) => {
      listeners.delete(listener)
    },
    // Fire every registered visibilitychange listener, like the browser does.
    fireVisibilityChange: () => {
      for (const listener of [...listeners]) listener()
    },
    listenerCount: () => listeners.size,
  }
  vi.stubGlobal('document', doc)
  return doc
}

const doc = stubDocument()

function setVisibility(state: 'visible' | 'hidden') {
  doc.hidden = state === 'hidden'
  doc.fireVisibilityChange()
}

describe('createVisibilityPoll', () => {
  const handles: VisibilityPollHandle[] = []

  function poll(tick: () => void, options: Parameters<typeof createVisibilityPoll>[1]) {
    const handle = createVisibilityPoll(tick, options)
    handles.push(handle)
    return handle
  }

  afterEach(() => {
    for (const handle of handles.splice(0)) handle.clear()
    setVisibility('visible')
    vi.useRealTimers()
  })

  it('ticks at the visible cadence', () => {
    vi.useFakeTimers()
    const tick = vi.fn()
    poll(tick, { intervalMs: 3000 })
    expect(tick).not.toHaveBeenCalled()
    vi.advanceTimersByTime(3000)
    expect(tick).toHaveBeenCalledTimes(1)
    vi.advanceTimersByTime(6000)
    expect(tick).toHaveBeenCalledTimes(3)
  })

  it('pauses while hidden when no hidden cadence is given, refreshes immediately on return', () => {
    vi.useFakeTimers()
    const tick = vi.fn()
    poll(tick, { intervalMs: 3000 })
    setVisibility('hidden')
    vi.advanceTimersByTime(30_000)
    expect(tick).not.toHaveBeenCalled()
    setVisibility('visible')
    expect(tick).toHaveBeenCalledTimes(1) // immediate refresh on return
    vi.advanceTimersByTime(6000)
    expect(tick).toHaveBeenCalledTimes(3) // normal cadence resumed
  })

  it('throttles to the hidden cadence while hidden instead of pausing', () => {
    vi.useFakeTimers()
    const tick = vi.fn()
    poll(tick, { intervalMs: 10_000, hiddenIntervalMs: 60_000 })
    setVisibility('hidden')
    vi.advanceTimersByTime(10_000)
    expect(tick).not.toHaveBeenCalled() // the visible cadence no longer applies
    vi.advanceTimersByTime(50_000)
    expect(tick).toHaveBeenCalledTimes(1) // 60s hidden cadence
    setVisibility('visible')
    expect(tick).toHaveBeenCalledTimes(2) // immediate refresh on return
    vi.advanceTimersByTime(10_000)
    expect(tick).toHaveBeenCalledTimes(3) // back to the 10s cadence
  })

  it('starting hidden arms the hidden cadence (or pause) from the start', () => {
    vi.useFakeTimers()
    setVisibility('hidden')
    const paused = vi.fn()
    const throttled = vi.fn()
    poll(paused, { intervalMs: 3000 })
    poll(throttled, { intervalMs: 10_000, hiddenIntervalMs: 60_000 })
    vi.advanceTimersByTime(59_000)
    expect(paused).not.toHaveBeenCalled()
    expect(throttled).not.toHaveBeenCalled()
    vi.advanceTimersByTime(1000)
    expect(paused).not.toHaveBeenCalled()
    expect(throttled).toHaveBeenCalledTimes(1)
  })

  it('skips the immediate refresh when refreshOnVisible is false', () => {
    vi.useFakeTimers()
    const tick = vi.fn()
    poll(tick, { intervalMs: 3000, refreshOnVisible: false })
    setVisibility('hidden')
    setVisibility('visible')
    expect(tick).not.toHaveBeenCalled()
    vi.advanceTimersByTime(3000)
    expect(tick).toHaveBeenCalledTimes(1)
  })

  it('clear() stops the timer and removes the listener; it is idempotent', () => {
    vi.useFakeTimers()
    const tick = vi.fn()
    const handle = poll(tick, { intervalMs: 3000, hiddenIntervalMs: 60_000 })
    expect(doc.listenerCount()).toBe(1)
    handle.clear()
    handle.clear()
    expect(doc.listenerCount()).toBe(0)
    setVisibility('hidden')
    setVisibility('visible')
    vi.advanceTimersByTime(120_000)
    expect(tick).not.toHaveBeenCalled()
  })

  it('does not resurrect a handle retired synchronously by its own tick', () => {
    vi.useFakeTimers()
    const tick = vi.fn()
    let handle!: VisibilityPollHandle
    tick.mockImplementation(() => handle.clear())
    handle = poll(tick, { intervalMs: 3000 })
    setVisibility('hidden')
    setVisibility('visible') // immediate tick retires the handle mid-callback
    expect(tick).toHaveBeenCalledTimes(1)
    vi.advanceTimersByTime(30_000)
    expect(tick).toHaveBeenCalledTimes(1) // no resurrected timer
  })
})
