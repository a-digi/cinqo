// paginationRules.ts — the pure rules of the pagination bar (plan 259): how many pages there are, which page numbers are shown, and what the "Showing …" line says.

export const DEFAULT_PAGE_SIZES = [10, 20, 50] as const

// How many pages hold `total` rows of `size`; a list with no rows has one (empty) page.
export function totalPages(total: number, size: number): number {
  if (size < 1) return 1
  return Math.max(1, Math.ceil(total / size))
}

// A page number put inside 1..last (a page past the last one in the address shows the last page).
export function clampPage(page: number, total: number, size: number): number {
  const last = totalPages(total, size)
  if (!Number.isFinite(page) || page < 1) return 1
  return Math.min(Math.floor(page), last)
}

export type PageItem = number | 'gap'

// The buttons of the bar: all the pages when there are at most 7, otherwise always the first and the last, the neighbours of the current page and "gap" marks
// for what is left out, so the bar is always 7 buttons wide and does not jump while the person pages.
export function pageWindow(current: number, last: number): PageItem[] {
  if (last <= 7) return Array.from({ length: last }, (_, i) => i + 1)
  if (current <= 4) return [1, 2, 3, 4, 5, 'gap', last]
  if (current >= last - 3) return [1, 'gap', last - 4, last - 3, last - 2, last - 1, last]
  return [1, 'gap', current - 1, current, current + 1, 'gap', last]
}

// "Showing 21–40 of 87"; "Nothing to show" for an empty list.
export function rangeText(page: number, size: number, total: number): string {
  if (total <= 0) return 'Nothing to show'
  const from = (page - 1) * size + 1
  const to = Math.min(page * size, total)
  if (from > total) return `Nothing on this page, ${String(total)} in all`
  return `Showing ${String(from)}–${String(to)} of ${String(total)}`
}
