// The geometry of the summary rail's ring: turns segment values into stroke dashes
// around a circle. Pure (no React), so it can be script-checked.
export interface RingSegment {
  value: number
  color: string
}

export interface RingArc {
  color: string
  // The visible stretch of the circle's circumference, in the same unit as `circumference`.
  dash: number
  // Where it starts, as a negative stroke-dashoffset (so arcs follow one another clockwise).
  offset: number
}

// Segments with a value of zero (or less, or not a number) are skipped. No positive value at
// all gives no arcs, and the caller draws just the empty track.
export function ringArcs(segments: RingSegment[], circumference: number): RingArc[] {
  const usable = segments.filter((s) => Number.isFinite(s.value) && s.value > 0)
  const total = usable.reduce((sum, s) => sum + s.value, 0)
  if (total === 0) return []
  const arcs: RingArc[] = []
  let used = 0
  for (const s of usable) {
    const dash = (s.value / total) * circumference
    arcs.push({ color: s.color, dash: Math.round(dash * 100) / 100, offset: -Math.round(used * 100) / 100 })
    used += dash
  }
  return arcs
}
