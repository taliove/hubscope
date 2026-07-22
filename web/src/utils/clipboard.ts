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
  el.style.opacity = '0'
  document.body.appendChild(el)
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
