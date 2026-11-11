// templatePaging.ts (plan 250, step 1) — the pure parts of the paged template picker: how many templates a page holds, which page a number or a template
// belongs to, and a page of a list. No React in here; TemplateStep keeps the current page in state and asks these.

// Twelve templates a page: three rows of four from the `md` breakpoint up, and still full rows with two or three columns (12 divides by 2, 3 and 4).
export const TEMPLATES_PER_PAGE = 12

// How many pages a list of `total` templates takes (an empty list is one empty page, never zero).
export function pageCount(total: number): number {
  return Math.max(1, Math.ceil(total / TEMPLATES_PER_PAGE))
}

// A page number that exists for a list of `total` templates: not below 1, not above the last page, and 1 for anything that is not a number.
export function clampPage(page: number, total: number): number {
  const p = Number.isFinite(page) ? Math.floor(page) : 1
  return Math.min(Math.max(1, p), pageCount(total))
}

// The templates of one page of the list (the page is clamped, so a page that no longer exists shows the last one).
export function pageSlice<T>(list: readonly T[], page: number): T[] {
  const p = clampPage(page, list.length)
  return list.slice((p - 1) * TEMPLATES_PER_PAGE, p * TEMPLATES_PER_PAGE)
}

// The page of the list that holds the template with this id; 1 when it is not in the list (nothing chosen yet, or filtered out).
export function pageOf(list: readonly { id: string }[], id: string): number {
  const i = list.findIndex((t) => t.id === id)
  return i < 0 ? 1 : Math.floor(i / TEMPLATES_PER_PAGE) + 1
}
