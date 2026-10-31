// Pure rules for the shared Carousel (step 105): which slide a step or a swipe leads to, where a
// slide sits, which key means which direction, how a slide is announced, and when auto-play may
// run. No React, so they can be script-checked. See
// plan/ai/tools/career/step-105-shared-carousel-and-portal-tips.md.

export type Direction = 'next' | 'prev'
export type Orientation = 'horizontal' | 'vertical'

// An index kept inside 0 .. count - 1 (0 when there are no slides or the value is not a number).
export function clampIndex(index: number, count: number): number {
  if (count <= 0 || !Number.isFinite(index)) return 0
  return Math.min(count - 1, Math.max(0, Math.trunc(index)))
}

// Where "next" or "previous" leads from index. Without loop it stays put at an end; with loop it wraps.
export function stepIndex(index: number, count: number, direction: Direction, loop: boolean): number {
  if (count <= 1) return 0
  const here = clampIndex(index, count)
  const target = direction === 'next' ? here + 1 : here - 1
  if (target < 0) return loop ? count - 1 : 0
  if (target > count - 1) return loop ? 0 : count - 1
  return target
}

// How far slide i is from the one shown: 0 for the shown slide, negative for the ones before it
// (they wait at minus 100% of the frame), positive for the ones after (plus 100%). The component
// turns this into a move on the Y axis (vertical) or the X axis (horizontal).
export function slideOffset(i: number, index: number): number {
  if (!Number.isFinite(i) || !Number.isFinite(index)) return 0
  return Math.trunc(i) - Math.trunc(index)
}

// What a finished drag means: a clear HORIZONTAL movement of at least threshold pixels. Dragging to
// the left moves on ("next"), like turning a page. A mostly vertical gesture is a scroll, not a
// swipe. Only the horizontal carousel uses this: on a vertical one a vertical drag is the page scrolling.
export function swipeDirection(dx: number, dy: number, threshold = 50): Direction | null {
  if (!Number.isFinite(dx) || !Number.isFinite(dy)) return null
  if (Math.abs(dx) < threshold || Math.abs(dx) <= Math.abs(dy)) return null
  return dx < 0 ? 'next' : 'prev'
}

// What a key means for a carousel of this orientation: Right/Left for a horizontal one, Down/Up for
// a vertical one. Any other key, and the arrows of the other axis, mean nothing here (so the page
// keeps them).
export function keyDirection(key: string, orientation: Orientation): Direction | null {
  if (orientation === 'horizontal') {
    if (key === 'ArrowRight') return 'next'
    if (key === 'ArrowLeft') return 'prev'
    return null
  }
  if (key === 'ArrowDown') return 'next'
  if (key === 'ArrowUp') return 'prev'
  return null
}

// "2 of 4" or "2 of 4: Paywalls": what a screen reader hears for a slide.
export function slideLabel(index: number, count: number, label?: string): string {
  const position = `${clampIndex(index, count) + 1} of ${Math.max(count, 1)}`
  return label && label.trim() !== '' ? `${position}: ${label.trim()}` : position
}

// The time between automatic slides, or 0 when it must not run: switched off, not enough slides,
// the user asked for less motion, or the user is reading (pointer or focus inside, or the pause button pressed).
export const MIN_AUTOPLAY_MS = 2000

export function autoPlayInterval(ms: number, count: number, reducedMotion: boolean, paused: boolean): number {
  if (!Number.isFinite(ms) || ms <= 0 || count <= 1 || reducedMotion || paused) return 0
  return Math.max(MIN_AUTOPLAY_MS, ms)
}

// The usual times between automatic slides (step 109). Any other number works too, from MIN_AUTOPLAY_MS up.
export const AUTO_PLAY_MS = { fast: 3000, normal: 5000, slow: 8000 } as const

// Whether the pause button is worth showing: only when the carousel would move by itself (it is switched on, there
// is more than one slide, and the user has not asked for less motion). Whether it is paused right now does not matter.
export function showsPauseButton(ms: number, count: number, reducedMotion: boolean): boolean {
  return autoPlayInterval(ms, count, reducedMotion, false) > 0
}
