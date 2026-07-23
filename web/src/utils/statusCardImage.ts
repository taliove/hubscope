// DOM→PNG capture for the StatusCard (ticket 49). Pure client-side via
// snapdom; no backend involvement (W8: the dependency is bundled by vite,
// the binary stays self-contained). snapdom is dynamically imported so its
// ~100KB stays out of the Dashboard chunk until someone actually shares.
// The captured node must live in the current document so scoped styles and
// CSS variables resolve. The dialog captures an offscreen twin rather than
// the visible preview: snapdom applies ancestor overflow clipping to the
// output, and the preview caps its height for scrolling — capturing it
// would clip the PNG to the visible slice.

// Logical card width is 720px; exporting at 2x keeps the PNG crisp on
// Retina screens and in chat apps that upscale previews.
export const STATUS_CARD_EXPORT_SCALE = 2

// hubscope-status-YYYYMMDD-HHmm.png
export function statusCardFilename(now: Date): string {
  const pad = (n: number) => String(n).padStart(2, '0')
  const date = `${now.getFullYear()}${pad(now.getMonth() + 1)}${pad(now.getDate())}`
  return `hubscope-status-${date}-${pad(now.getHours())}${pad(now.getMinutes())}.png`
}

async function capture(el: HTMLElement) {
  const { snapdom } = await import('@zumer/snapdom')
  // No backgroundColor option: the card root paints its own opaque ground
  // (var(--hs-bg-card)), so the PNG gets the designed surface for free.
  return snapdom(el, { scale: STATUS_CARD_EXPORT_SCALE })
}

// captureStatusCard renders the element once and returns a PNG blob
// (clipboard path).
export async function captureStatusCard(el: HTMLElement): Promise<Blob> {
  const result = await capture(el)
  return result.toBlob({ type: 'png' })
}

// downloadStatusCard renders the element and triggers a browser download.
// snapdom uses the given filename verbatim, so it must include the
// extension.
export async function downloadStatusCard(el: HTMLElement, filename: string): Promise<void> {
  const result = await capture(el)
  await result.download({ format: 'png', filename })
}
