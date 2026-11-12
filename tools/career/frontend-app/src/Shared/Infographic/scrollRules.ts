// Helpers the infographic shells share (plan 262, 267): which element scrolls a poster, whether the person asked for less motion, and where the
// sticky bar's own height ends. No React.

// The element that scrolls the poster: the nearest ancestor that scrolls (the body of the full-page modal).
export function scrollParentOf(el: HTMLElement | null): HTMLElement | null {
  for (let p = el?.parentElement ?? null; p; p = p.parentElement) {
    const overflow = getComputedStyle(p).overflowY
    if (overflow === 'auto' || overflow === 'scroll') return p
  }
  return null
}

export function prefersReducedMotion(): boolean {
  return (
    typeof window !== 'undefined' &&
    typeof window.matchMedia === 'function' &&
    window.matchMedia('(prefers-reduced-motion: reduce)').matches
  )
}

// Where the chapter bar's own height ends: a chapter counts as "in view" once its top has passed this line.
export const BAR_LINE = 96

// Where a sideways strip (the tabs) should scroll so that one item shows fully: the same position when the item is already inside the strip, else the
// position that centres it, never below 0 and never past the end. Widths in px; `itemLeft` is measured from the start of the scrolled content.
export function stripScrollFor(scrollLeft: number, viewWidth: number, contentWidth: number, itemLeft: number, itemWidth: number): number {
  if (itemLeft >= scrollLeft && itemLeft + itemWidth <= scrollLeft + viewWidth) return scrollLeft
  const centred = itemLeft + itemWidth / 2 - viewWidth / 2
  return Math.max(0, Math.min(centred, Math.max(0, contentWidth - viewWidth)))
}
