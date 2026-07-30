// Choreography table for the endpoint card flip + quick-view dialog
// (2026-07-29 /impeccable animate, dashboard surface brief).
//
// DESIGN.md Motion: choreography delays (stagger/overlap) never enter the
// token scale — they are single-use constants, single-sourced HERE with
// cross-referencing comments. The CSS twins that must change together:
//   - EndpointCard.vue          .is-flipped rotateY(-90deg), transform on the
//                               default 0.2s stop (QUICKVIEW_CARD_FLIP_MS)
//   - ep-theme.css              hs-flip enter/leave + the 0.12s delayed overlay
//                               leave fade (dialog flip on the focal 0.32s stop,
//                               QUICKVIEW_DIALOG_FLIP_MS)
//   - DashboardView.vue /       .card-grid perspective 1600px
//     OverviewGroupSection.vue  (QUICKVIEW_FLIP_PERSPECTIVE_PX)
//   - ep-theme.css              .hs-quickview-overlay backdrop-filter blur 8px
//                               (QUICKVIEW_OVERLAY_BLUR_PX)

// Durations mirror the two transition stops in tokens.css.
export const QUICKVIEW_CARD_FLIP_MS = 200
export const QUICKVIEW_DIALOG_FLIP_MS = 320
// Open: the card flips out first; the frosted overlay + dialog flip-in start
// after this stagger, so open totals 140 + 320 = 460ms (the ≤460ms budget).
export const QUICKVIEW_OPEN_STAGGER_MS = 140
// Close (mirror of open): the dialog flips out while the card flip-back is
// allowed to overlap the tail of it — 60–80ms per the design ruling, so close
// totals (320 − 70) + 200 = 450ms, never stickier than opening.
export const QUICKVIEW_CLOSE_OVERLAP_MS = 70
export const QUICKVIEW_FLIP_PERSPECTIVE_PX = 1600
export const QUICKVIEW_OVERLAY_BLUR_PX = 8

export interface QuickViewChoreo {
  // Delay from card click to opening the dialog (card flip-out leads).
  openDialogDelayMs: number
  // Delay from close start (EP @close, leave begins) to the card flip-back.
  closeCardDelayMs: number
  totalOpenMs: number
  totalCloseMs: number
}

// Phase timing as a pure function (ui-guidelines §6 JS 阶段门控条): the global
// `transition: none` reduced-motion fallback zeroes CSS but NOT setTimeout
// stage delays — under reduced motion every delay must be zero and the final
// state presented immediately, otherwise the delay itself reads as jank.
export function quickViewChoreo(reducedMotion: boolean): QuickViewChoreo {
  if (reducedMotion) {
    return { openDialogDelayMs: 0, closeCardDelayMs: 0, totalOpenMs: 0, totalCloseMs: 0 }
  }
  const openDialogDelayMs = QUICKVIEW_OPEN_STAGGER_MS
  const closeCardDelayMs = QUICKVIEW_DIALOG_FLIP_MS - QUICKVIEW_CLOSE_OVERLAP_MS
  return {
    openDialogDelayMs,
    closeCardDelayMs,
    totalOpenMs: openDialogDelayMs + QUICKVIEW_DIALOG_FLIP_MS,
    totalCloseMs: closeCardDelayMs + QUICKVIEW_CARD_FLIP_MS,
  }
}
