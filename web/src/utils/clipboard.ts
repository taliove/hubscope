// copyText writes text to the clipboard, falling back to a hidden textarea
// on non-secure contexts where navigator.clipboard is unavailable. Returns
// whether the copy succeeded so callers can surface a manual-copy fallback.
export async function copyText(text: string): Promise<boolean> {
  try {
    await navigator.clipboard.writeText(text)
    return true
  } catch {
    // Fall through to the textarea fallback.
  }
  const el = document.createElement('textarea')
  el.value = text
  el.style.position = 'fixed'
  el.style.top = '0'
  el.style.left = '0'
  el.style.opacity = '0'
  document.body.appendChild(el)
  // Focus before select — without it execCommand('copy') has no selection
  // target in several engines (and silently returns false).
  el.focus()
  el.select()
  let ok = false
  try {
    ok = document.execCommand('copy')
  } catch {
    ok = false
  }
  document.body.removeChild(el)
  return ok
}

// canCopyImage reports whether image clipboard writes are available: they
// require a secure context (HTTPS/localhost), navigator.clipboard.write and
// ClipboardItem. When false, callers must degrade to a download path.
export function canCopyImage(): boolean {
  return (
    typeof window !== 'undefined' &&
    window.isSecureContext &&
    typeof navigator !== 'undefined' &&
    'clipboard' in navigator &&
    typeof navigator.clipboard.write === 'function' &&
    typeof ClipboardItem !== 'undefined'
  )
}

// copyImageBlob writes an image blob to the clipboard. Returns false when
// unsupported or rejected (permission denied, browser quirks) so callers
// can point the user at the download fallback.
export async function copyImageBlob(blob: Blob): Promise<boolean> {
  if (!canCopyImage()) return false
  try {
    await navigator.clipboard.write([new ClipboardItem({ [blob.type]: blob })])
    return true
  } catch {
    return false
  }
}
