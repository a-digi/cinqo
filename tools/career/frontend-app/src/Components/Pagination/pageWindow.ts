// pageWindow.ts (plan 250, step 2) — which page numbers a numbered pager shows: the first and the last, the current one and its two neighbours, and a gap where
// pages are left out. A gap that would hide only one page shows that page instead (a "…" for a single number is no saving).
export type PageSlot = number | 'gap'

export function pageWindow(current: number, count: number): PageSlot[] {
  const last = Math.max(1, Math.floor(count))
  const here = Math.min(Math.max(1, Math.floor(current)), last)
  const wanted = [...new Set([1, last, here - 1, here, here + 1])].filter((n) => n >= 1 && n <= last).sort((a, b) => a - b)
  const out: PageSlot[] = []
  let before = 0
  for (const n of wanted) {
    if (before > 0) {
      if (n - before === 2) out.push(before + 1)
      else if (n - before > 2) out.push('gap')
    }
    out.push(n)
    before = n
  }
  return out
}
