// Which decorative shape sits behind a card (step 106): the same three the dashboards use, so a row
// of cards is not a wall of identical rectangles. Pure (no React), so it can be script-checked.

export type DecorShape = 'circles' | 'diamond' | 'ring'

export const DECOR_SHAPES: readonly DecorShape[] = ['circles', 'diamond', 'ring']

// The shape for the n-th card in a row: they take turns, whatever n is (a stray or negative n still gets one).
export function shapeAt(n: number): DecorShape {
  const i = Number.isFinite(n) ? Math.trunc(n) : 0
  return DECOR_SHAPES[((i % DECOR_SHAPES.length) + DECOR_SHAPES.length) % DECOR_SHAPES.length]
}
