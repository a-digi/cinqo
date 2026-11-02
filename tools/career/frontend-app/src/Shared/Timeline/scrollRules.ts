// Pure rules of the scrolling timeline (plan/ai/tools/career/step-170-onboarding-guide.md): what can still be scrolled, how far, how a mouse
// drag moves it, and the edge fade. No React, so they can be script-checked. `client` is the visible width, `total` the whole width.

export interface ScrollEdges {
  canLeft: boolean
  canRight: boolean
}

// Whether there is more to see to the left and to the right; a pixel or two of rounding does not count.
export function scrollEdges(left: number, client: number, total: number, tolerance = 2): ScrollEdges {
  return { canLeft: left > tolerance, canRight: left + client < total - tolerance }
}

export function maxScroll(client: number, total: number): number {
  return Math.max(0, total - client)
}

export function clampScroll(target: number, client: number, total: number): number {
  return Math.min(Math.max(target, 0), maxScroll(client, total))
}

// How far along the scroll is, from 0 (left end) to 1 (right end); 0 when there is nothing to scroll.
export function scrollProgress(left: number, client: number, total: number): number {
  const max = maxScroll(client, total)
  return max === 0 ? 0 : Math.min(Math.max(left / max, 0), 1)
}

// The share of the whole width that is visible (the width of the progress thumb); 1 when everything is visible.
export function thumbFraction(client: number, total: number): number {
  return total <= 0 || total <= client ? 1 : client / total
}

// Where a click on a prev / next button scrolls to: most of a screen at a time, so the last visible step stays in view.
export function pageTarget(left: number, client: number, total: number, direction: -1 | 1): number {
  return clampScroll(left + direction * client * 0.8, client, total)
}

// A press that moved more than a few pixels is a drag, not a click.
export function isDrag(startX: number, x: number, threshold = 5): boolean {
  return Math.abs(x - startX) > threshold
}

// Dragging with the mouse moves the content with the pointer: the scroll goes the other way.
export function dragTarget(startLeft: number, startX: number, x: number, client: number, total: number): number {
  return clampScroll(startLeft - (x - startX), client, total)
}

// The mask that fades the content out at an edge that has more behind it (none when everything is visible).
export function edgeMask(edges: ScrollEdges, fadePx = 48): string {
  if (!edges.canLeft && !edges.canRight) return 'none'
  const left = edges.canLeft ? `transparent 0, #000 ${String(fadePx)}px` : '#000 0'
  const right = edges.canRight ? `#000 calc(100% - ${String(fadePx)}px), transparent 100%` : '#000 100%'
  return `linear-gradient(to right, ${left}, ${right})`
}
