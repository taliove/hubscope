// DOM→PNG capture for the share cards (ticket 56 StatusCard, generalized in
// ticket 76 for the EvalCard). Pure client-side via snapdom; no backend
// involvement (W8: the dependency is bundled by vite, the binary stays
// self-contained). snapdom is dynamically imported so its ~100KB stays out
// of the Dashboard chunk until someone actually shares. The captured node
// must live in the current document so scoped styles and CSS variables
// resolve. The dialog captures an offscreen twin rather than the visible
// preview: snapdom applies ancestor overflow clipping to the output, and the
// preview caps its height for scrolling — capturing it would clip the PNG
// to the visible slice.

// Logical card width is 720px; exporting at 2x keeps the PNG crisp on
// Retina screens and in chat apps that upscale previews.
export const CARD_EXPORT_SCALE = 2

// Card kinds sharing this capture path. The kind prefixes the filename so
// status and eval shares sort apart in a download folder.
export type CardKind = 'status' | 'eval'

// Variant type for 480px compact version (GH #93).
export type CardVariant = 'full' | 'compact'

// hubscope-{kind}[-scope-][-compact-]YYYYMMDD-HHmm.png — scope (e.g. a group
// key for status, "批次N" for eval) lets saved scoped shares sort apart from
// the unscoped one; compact appends the variant segment. Characters outside
// letters/digits/CJK/-/_ collapse to underscores. Full-width variant preserves
// the established filename convention (no -full segment).
export function cardFilename(now: Date, kind: CardKind, scope?: string, variant: CardVariant = 'full'): string {
  const pad = (n: number) => String(n).padStart(2, '0')
  const date = `${now.getFullYear()}${pad(now.getMonth() + 1)}${pad(now.getDate())}`
  const scopePart = scope ? `-${scope.replace(/[^\w一-鿿.-]+/g, '_')}` : ''
  const variantPart = variant === 'compact' ? '-compact' : ''
  return `hubscope-${kind}${scopePart}${variantPart}-${date}-${pad(now.getHours())}${pad(now.getMinutes())}.png`
}

async function capture(el: HTMLElement) {
  const { snapdom } = await import('@zumer/snapdom')
  // No backgroundColor option: the card root paints its own opaque ground
  // (var(--hs-bg-card)), so the PNG gets the designed surface for free.
  return snapdom(el, { scale: CARD_EXPORT_SCALE })
}

// captureCardImage renders the element once and returns a PNG blob
// (clipboard path).
export async function captureCardImage(el: HTMLElement): Promise<Blob> {
  const result = await capture(el)
  return result.toBlob({ type: 'png' })
}

// downloadCardImage renders the element and triggers a browser download.
// snapdom uses the given filename verbatim, so it must include the
// extension.
export async function downloadCardImage(el: HTMLElement, filename: string): Promise<void> {
  const result = await capture(el)
  await result.download({ format: 'png', filename })
}
