import { DEFAULT_WINDOW, WINDOW_SIZES, type WindowSize } from './terminalRules'

// Per-viewer conveniences kept in the browser: the window size and whether long lines wrap. Storage can be
// blocked or empty (private windows, cleared data), so every read and write is guarded and the viewer works
// the same without it.
const SIZE_KEY = 'cinqo:terminal:window'
const WRAP_KEY = 'cinqo:terminal:wrap'

export function readWindowSize(fallback: WindowSize = DEFAULT_WINDOW): WindowSize {
  try {
    const n = Number(window.localStorage.getItem(SIZE_KEY))
    return WINDOW_SIZES.find((s) => s === n) ?? fallback
  } catch {
    return fallback
  }
}

export function saveWindowSize(size: WindowSize): void {
  try {
    window.localStorage.setItem(SIZE_KEY, String(size))
  } catch {
    // no storage: the choice just is not remembered
  }
}

export function readWrap(): boolean {
  try {
    return window.localStorage.getItem(WRAP_KEY) === '1'
  } catch {
    return false
  }
}

export function saveWrap(wrap: boolean): void {
  try {
    window.localStorage.setItem(WRAP_KEY, wrap ? '1' : '0')
  } catch {
    // no storage: the choice just is not remembered
  }
}
