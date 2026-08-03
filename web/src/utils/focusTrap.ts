// Minimal focus trap for self-built modal surfaces (GH #116 check MEDIUM-2,
// spec 0018 decision 8). el-dialog ships its own focus-trap; the v2
// signature surfaces are self-built, so every self-built modal (the detail
// panel first, the rebuilt share dialogs next) installs this trap to keep
// Tab / Shift+Tab cycling INSIDE the surface — without it an aria-modal
// surface leaks focus to the content behind its own scrim.
//
// One third of the self-built modal trio (to be registered in the surface
// briefs at batch end): focus trap + unified ESC/scrim close + focus
// return. The panel owns the latter two; this module owns the first.
//
// Structure: the wrap DECISION is a DOM-free pure function (trapTabTarget,
// vitest-covered); the DOM wrapper is a thin query + listener shell.

const FOCUSABLE_SELECTOR =
  'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])'

// The Tab-wrap decision: returns the element that must RECEIVE focus (and
// the event must be hijacked), or null when the browser's default Tab move
// already lands inside the list. Focus escaping the container (active not
// in the list — e.g. the browser chrome cycle re-entering the page) wraps
// to the boundary element in the direction of travel.
export function trapTabTarget<T>(active: T | null, focusables: readonly T[], shiftKey: boolean): T | null {
  if (focusables.length === 0) return null
  const first = focusables[0]
  const last = focusables[focusables.length - 1]
  const inside = active !== null && focusables.includes(active)
  if (shiftKey) {
    return !inside || active === first ? last : null
  }
  return !inside || active === last ? first : null
}

// Visible focusable descendants in DOM order. Visibility is measured by
// getClientRects (offsetParent lies for fixed-position overlays — exactly
// what modal surfaces are).
export function getFocusableElements(container: HTMLElement): HTMLElement[] {
  return Array.from(container.querySelectorAll<HTMLElement>(FOCUSABLE_SELECTOR)).filter(
    el => el.getClientRects().length > 0,
  )
}

export interface FocusTrapHandle {
  deactivate(): void
}

// Install the trap: a document-level keydown listener that hijacks Tab /
// Shift+Tab when focus would leave the container. An empty focusable list
// swallows Tab outright (a surface with nothing focusable must still not
// leak). deactivate() removes the listener; pairing is the installer's
// duty (same discipline as the ESC listener).
export function createFocusTrap(container: HTMLElement): FocusTrapHandle {
  function onKeydown(e: KeyboardEvent) {
    if (e.key !== 'Tab') return
    const focusables = getFocusableElements(container)
    if (focusables.length === 0) {
      e.preventDefault()
      return
    }
    const target = trapTabTarget(document.activeElement as HTMLElement | null, focusables, e.shiftKey)
    if (target !== null) {
      e.preventDefault()
      target.focus()
    }
  }
  document.addEventListener('keydown', onKeydown)
  return {
    deactivate() {
      document.removeEventListener('keydown', onKeydown)
    },
  }
}
