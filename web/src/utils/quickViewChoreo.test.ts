// Unit tests for the quick-view flip choreography table: the motion branch
// must reproduce the designed timings (140ms stagger, 60–80ms close overlap,
// both directions within the ≤460ms budget), and the reduced-motion branch
// must zero every stage delay (ui-guidelines §6 JS 阶段门控条 — delays are
// waiting, not decoration).
import { describe, it, expect } from 'vitest'
import {
  quickViewChoreo,
  QUICKVIEW_CARD_FLIP_MS,
  QUICKVIEW_DIALOG_FLIP_MS,
  QUICKVIEW_OPEN_STAGGER_MS,
  QUICKVIEW_CLOSE_OVERLAP_MS,
} from '@/utils/quickViewChoreo'

describe('quickViewChoreo motion branch', () => {
  const choreo = quickViewChoreo(false)

  it('opens with the 140ms stagger and stays within the 460ms budget', () => {
    expect(choreo.openDialogDelayMs).toBe(QUICKVIEW_OPEN_STAGGER_MS)
    expect(choreo.openDialogDelayMs).toBe(140)
    expect(choreo.totalOpenMs).toBe(QUICKVIEW_OPEN_STAGGER_MS + QUICKVIEW_DIALOG_FLIP_MS)
    expect(choreo.totalOpenMs).toBeLessThanOrEqual(460)
  })

  it('closes as the mirror image with a 60–80ms overlap, never stickier than open', () => {
    const overlap = QUICKVIEW_DIALOG_FLIP_MS - choreo.closeCardDelayMs
    expect(overlap).toBe(QUICKVIEW_CLOSE_OVERLAP_MS)
    expect(overlap).toBeGreaterThanOrEqual(60)
    expect(overlap).toBeLessThanOrEqual(80)
    expect(choreo.totalCloseMs).toBe(choreo.closeCardDelayMs + QUICKVIEW_CARD_FLIP_MS)
    expect(choreo.totalCloseMs).toBeLessThanOrEqual(460)
  })
})

describe('quickViewChoreo reduced-motion branch', () => {
  it('zeroes every stage delay and total', () => {
    expect(quickViewChoreo(true)).toEqual({
      openDialogDelayMs: 0,
      closeCardDelayMs: 0,
      totalOpenMs: 0,
      totalCloseMs: 0,
    })
  })
})
