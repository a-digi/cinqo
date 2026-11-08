// Page numbers and page sizes in a list page's address (steps 135, 137, 138): how they are read and how a page is kept inside
// the list. Shared so the Jobs and Companies lists cannot drift apart.

// Digits accepted for a page number; more than this is not a page, it is the default (page 1).
const MAX_PAGE_DIGITS = 7

// A page number read from text or a number: a whole number of 1 to MAX_PAGE_DIGITS digits, at least 1; anything else
// (a sign, a decimal, an exponent, text, blank, too many digits) is page 1.
export function cleanPage(raw: string | null | number): number {
  const text = typeof raw === 'number' ? (Number.isInteger(raw) ? String(raw) : '') : (raw ?? '').trim()
  if (!new RegExp(`^[0-9]{1,${String(MAX_PAGE_DIGITS)}}$`).test(text)) return 1
  return Math.max(1, Number(text))
}

// The page to show once the total is known: never before the first, never past the last (a list of nothing has one page).
export function clampPage(page: number, total: number, pageSize: number): number {
  const pages = Math.max(1, Math.ceil(Math.max(0, total) / Math.max(1, pageSize)))
  return Math.min(Math.max(1, cleanPage(page)), pages)
}

// The range of a per-page box (the backend clamps to the same 1 to 200).
export const MIN_PAGE_LIMIT = 1
export const MAX_PAGE_LIMIT = 200

// A per-page value typed by hand: a whole number of at most three digits from 1 to 200, otherwise null (the caller keeps the
// value it had). Spaces around it are ignored; a sign, a decimal, an exponent and text are not numbers here.
export function limitFromText(raw: string | null): number | null {
  const text = (raw ?? '').trim()
  if (!/^[0-9]{1,3}$/.test(text)) return null
  const n = Number(text)
  return n >= MIN_PAGE_LIMIT && n <= MAX_PAGE_LIMIT ? n : null
}

// A per-page value read from the address or a state: itself when it is valid, otherwise the list's default.
export function cleanLimit(raw: string | null | number, fallback: number): number {
  const text = typeof raw === 'number' ? (Number.isInteger(raw) ? String(raw) : '') : raw
  return limitFromText(text) ?? fallback
}
