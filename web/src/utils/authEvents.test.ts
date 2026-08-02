// Unit tests for the auth-change event channel (GH #148). The load-bearing
// invariants: the event name is frozen at 'hs:auth-changed' (emit and listen
// sides share this single source), and dispatching reaches a listener
// registered under that name. The suite runs in the node environment, so
// `window` is stubbed with a real EventTarget — the same dispatch interface
// the browser window provides.
import { afterEach, describe, expect, it, vi } from 'vitest'
import { AUTH_CHANGED_EVENT, dispatchAuthChanged } from '@/utils/authEvents'

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('authEvents', () => {
  it('freezes the event name at hs:auth-changed', () => {
    expect(AUTH_CHANGED_EVENT).toBe('hs:auth-changed')
  })

  it('dispatchAuthChanged notifies a listener registered under the frozen name', () => {
    const target = new EventTarget()
    vi.stubGlobal('window', target)
    const listener = vi.fn()
    target.addEventListener(AUTH_CHANGED_EVENT, listener)

    dispatchAuthChanged()

    expect(listener).toHaveBeenCalledTimes(1)
  })

  it('dispatchAuthChanged dispatches exactly the frozen event type', () => {
    const seen: string[] = []
    vi.stubGlobal('window', {
      dispatchEvent: (ev: Event) => {
        seen.push(ev.type)
        return true
      },
    })

    dispatchAuthChanged()

    expect(seen).toEqual(['hs:auth-changed'])
  })
})
