// Pure rules of the pinned stage (plan 269): which step is shown for how far the person has scrolled through the stage. No React.

const clamp = (n: number, lo: number, hi: number): number => Math.min(hi, Math.max(lo, n))

// The step (0-based) for a progress from 0 (the stage just reached the top) to 1 (the stage is about to leave): the steps share the progress equally; a progress outside 0..1 or not a number
// is the nearest end (or the first step), and `count` below one is step 0.
export function pinnedStep(progress: number, count: number): number {
  if (!Number.isFinite(count) || count < 1) return 0
  const p = Number.isFinite(progress) ? clamp(progress, 0, 1) : 0
  return Math.min(Math.floor(count) - 1, Math.floor(p * Math.floor(count)))
}

// How far through a tall section the person is. `top` is the section's top relative to the top of the scroller (negative once it has scrolled past), `height` the section's height,
// `stageHeight` the height of the sticky stage inside it, `offset` where the stage sticks (its `top`). The section is "used up" after height - stageHeight of scrolling; a section that
// is not taller than its stage never moves (0).
export function pinnedProgress(top: number, height: number, stageHeight: number, offset = 0): number {
  const range = height - stageHeight
  if (!(range > 0)) return 0
  return clamp((offset - top) / range, 0, 1)
}

// The height of a pinned section for `count` steps: a screen of scrolling for each step and one for the stage itself, as a CSS length in viewport units.
export const STEP_VH = 70
export function pinnedSectionHeight(count: number): string {
  return `${String(Math.max(1, Math.floor(count)) * STEP_VH + 30)}vh`
}
