// Pure geometry of the radial timeline (plan/ai/tools/career/step-170-onboarding-guide.md): a ring split into one sector per step, a coloured
// band on the outside of each sector, a white wedge panel inside it and a hub in the middle. Everything is in a 1000 x 1000 box with y pointing
// down and angles in degrees measured clockwise from the right-hand direction. No React, so it can be script-checked.

export const RADIAL = {
  size: 1000,
  center: 500,
  bandOuter: 486,
  bandInner: 450,
  panelOuter: 446,
  panelInner: 146,
  hub: 130,
  // The empty angle between two neighbouring sectors, and the space kept between a panel's edge and its words.
  gapDeg: 2.4,
  textMargin: 10,
} as const

// The radial design needs enough steps to look like a ring and few enough for the words to fit in a wedge.
export const RADIAL_MIN_STEPS = 3
export const RADIAL_MAX_STEPS = 10

export function radialFits(count: number): boolean {
  return count >= RADIAL_MIN_STEPS && count <= RADIAL_MAX_STEPS
}

export interface Point {
  x: number
  y: number
}

export function polar(radius: number, deg: number): Point {
  const rad = (deg * Math.PI) / 180
  return { x: RADIAL.center + radius * Math.cos(rad), y: RADIAL.center + radius * Math.sin(rad) }
}

// The angles of a step's sector. The first sector is centred on the top and the others follow clockwise; the gap is split between the two
// sides, so the visible sector is a little narrower than its share of the circle.
export function sectorAngles(index: number, count: number): { start: number; end: number; mid: number } {
  const slice = 360 / Math.max(count, 1)
  const centre = -90 + index * slice
  return { start: centre - slice / 2 + RADIAL.gapDeg / 2, end: centre + slice / 2 - RADIAL.gapDeg / 2, mid: centre }
}

const fmt = (n: number) => n.toFixed(2)

// The outline of a ring segment between two radii and two angles (for a sector of less than half a circle).
export function annularSectorPath(inner: number, outer: number, startDeg: number, endDeg: number): string {
  const a = polar(outer, startDeg)
  const b = polar(outer, endDeg)
  const c = polar(inner, endDeg)
  const d = polar(inner, startDeg)
  const large = endDeg - startDeg > 180 ? 1 : 0
  return `M ${fmt(a.x)} ${fmt(a.y)} A ${fmt(outer)} ${fmt(outer)} 0 ${String(large)} 1 ${fmt(b.x)} ${fmt(b.y)} L ${fmt(c.x)} ${fmt(c.y)} A ${fmt(inner)} ${fmt(inner)} 0 ${String(large)} 0 ${fmt(d.x)} ${fmt(d.y)} Z`
}

export function bandPath(index: number, count: number): string {
  const { start, end } = sectorAngles(index, count)
  return annularSectorPath(RADIAL.bandInner, RADIAL.bandOuter, start, end)
}

export function panelPath(index: number, count: number): string {
  const { start, end } = sectorAngles(index, count)
  return annularSectorPath(RADIAL.panelInner, RADIAL.panelOuter, start, end)
}

// Where the label (the step's number) sits on its band.
export function labelPoint(index: number, count: number): Point {
  return polar((RADIAL.bandInner + RADIAL.bandOuter) / 2, sectorAngles(index, count).mid)
}

// Whether a point is inside a step's wedge panel, keeping `margin` away from every edge (the two straight sides and the two arcs).
export function insidePanel(p: Point, index: number, count: number, margin = 0): boolean {
  const dx = p.x - RADIAL.center
  const dy = p.y - RADIAL.center
  const r = Math.hypot(dx, dy)
  if (r < RADIAL.panelInner + margin || r > RADIAL.panelOuter - margin) return false
  const { start, end } = sectorAngles(index, count)
  const rs = (start * Math.PI) / 180
  const re = (end * Math.PI) / 180
  // The sector lies clockwise of its start side and anticlockwise of its end side (valid for sectors narrower than half a circle).
  const fromStart = Math.cos(rs) * dy - Math.sin(rs) * dx
  const fromEnd = Math.cos(re) * dy - Math.sin(re) * dx
  return fromStart >= margin && fromEnd <= -margin
}

export interface Rect {
  x: number
  y: number
  w: number
  h: number
}

function rectInside(rect: Rect, index: number, count: number, margin: number): boolean {
  const { x, y, w, h } = rect
  const stepX = Math.max(w / 6, 1)
  const stepY = Math.max(h / 6, 1)
  for (let i = 0; i <= 6; i++) {
    const px = x + i * stepX
    if (!insidePanel({ x: px, y }, index, count, margin) || !insidePanel({ x: px, y: y + h }, index, count, margin)) return false
  }
  for (let j = 1; j < 6; j++) {
    const py = y + j * stepY
    if (!insidePanel({ x, y: py }, index, count, margin) || !insidePanel({ x: x + w, y: py }, index, count, margin)) return false
  }
  return true
}

const rectCache = new Map<number, Rect[]>()

// The biggest upright rectangle for a step's words inside its wedge, kept off the wedge's edges and not too narrow or too flat (words need a
// readable measure). Found by trying sizes from big to small and the places where they might fit; the answer depends only on the step count.
export function textRects(count: number): Rect[] {
  const cached = rectCache.get(count)
  if (cached) return cached
  const sizes: { w: number; h: number }[] = []
  for (let w = 100; w <= 380; w += 10) {
    for (let h = 100; h <= 380; h += 10) {
      const aspect = w / h
      if (aspect >= 0.62 && aspect <= 1.6) sizes.push({ w, h })
    }
  }
  sizes.sort((a, b) => b.w * b.h - a.w * a.h)
  const rects: Rect[] = []
  for (let index = 0; index < count; index++) {
    const mid = polar((RADIAL.panelInner + RADIAL.panelOuter) / 2, sectorAngles(index, count).mid)
    let best: Rect = { x: mid.x - 50, y: mid.y - 50, w: 100, h: 100 }
    search: for (const size of sizes) {
      // Places around the middle of the wedge first, so the words sit in the centre of their panel when there is room.
      for (let ring = 0; ring <= 200; ring += 10) {
        for (let dy = -ring; dy <= ring; dy += 10) {
          for (let dx = -ring; dx <= ring; dx += 10) {
            if (Math.max(Math.abs(dx), Math.abs(dy)) !== ring) continue
            const rect = { x: mid.x - size.w / 2 + dx, y: mid.y - size.h / 2 + dy, w: size.w, h: size.h }
            if (rectInside(rect, index, count, RADIAL.textMargin)) {
              best = rect
              break search
            }
          }
        }
      }
    }
    rects.push({ x: Math.round(best.x), y: Math.round(best.y), w: best.w, h: best.h })
  }
  rectCache.set(count, rects)
  return rects
}

// The font size (in box units, so 1 unit is 1 px at a ring 1000 px wide) at which `chars` characters of running text fill a w x h rectangle:
// about half an em per character and 1.4 em per line, with a margin for the paragraph gaps. Never smaller than 9 nor bigger than 14.5.
export function fitFontSize(chars: number, w: number, h: number): number {
  const fit = Math.sqrt((0.9 * w * h) / (Math.max(chars, 1) * 0.54 * 1.4))
  return Math.min(Math.max(fit, 9), 14.5)
}

// The number of characters a step's words amount to (title, text, tip, link), a little more for the picture and the gaps.
export function wordsLength(step: { title: string; text: string; tip?: string; link?: { label: string } }): number {
  return (
    step.title.length * 1.3 +
    step.text.length +
    (step.tip === undefined ? 0 : step.tip.length + 8) +
    (step.link === undefined ? 0 : step.link.label.length + 8) +
    70
  )
}
