import { readonly, ref } from 'vue'

// Theme (light/dark) state for the app shell. Two-state toggle by design
// (v1 ruling, ui-guidelines §2a): default light, no system-preference
// following — the public status board is often projected/screenshotted and
// theme determinism wins. The choice persists to localStorage key
// `hs:dark`; the inline anti-FOUC script in index.html reads the SAME key
// before mount (keep the key name in sync between the two files).
const STORAGE_KEY = 'hs:dark'

const dark = ref(false)

function applyTheme(value: boolean) {
  dark.value = value
  document.documentElement.classList.toggle('dark', value)
}

export function useTheme() {
  function initTheme() {
    applyTheme(localStorage.getItem(STORAGE_KEY) === '1')
  }

  function toggleTheme() {
    const next = !dark.value
    try {
      localStorage.setItem(STORAGE_KEY, next ? '1' : '0')
    } catch {
      /* storage unavailable (private mode, quota) — theme still flips
         for the session, it just won't persist */
    }
    applyTheme(next)
  }

  return { dark: readonly(dark), initTheme, toggleTheme }
}
