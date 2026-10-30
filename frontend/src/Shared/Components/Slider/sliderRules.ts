// Pure rules for the shared Slider: which slide a step or a swipe leads to, how a slide is
// announced, and when auto-play may run. No React, so they can be script-checked.

export type Direction = 'next' | 'prev'

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

// What a finished drag means: a clear horizontal movement of at least threshold pixels. Dragging to
// the left moves on ("next"), like turning a page. A mostly vertical gesture is a scroll, not a swipe.
export function swipeDirection(dx: number, dy: number, threshold = 50): Direction | null {
  if (!Number.isFinite(dx) || !Number.isFinite(dy)) return null
  if (Math.abs(dx) < threshold || Math.abs(dx) <= Math.abs(dy)) return null
  return dx < 0 ? 'next' : 'prev'
}

// "2 of 4" or "2 of 4: OpenAI": what a screen reader hears for a slide.
export function slideLabel(index: number, count: number, label?: string): string {
  const position = `${clampIndex(index, count) + 1} of ${Math.max(count, 1)}`
  return label && label.trim() !== '' ? `${position}: ${label.trim()}` : position
}

// The time between automatic slides, or 0 when it must not run: switched off, not enough slides,
// the user asked for less motion, or the user is reading (pointer or focus inside).
export const MIN_AUTOPLAY_MS = 2000

export function autoPlayInterval(ms: number, count: number, reducedMotion: boolean, paused: boolean): number {
  if (!Number.isFinite(ms) || ms <= 0 || count <= 1 || reducedMotion || paused) return 0
  return Math.max(MIN_AUTOPLAY_MS, ms)
}
