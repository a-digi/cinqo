// scrollRules.ts — the pure rules of the live viewer's scrolling and keys: when the pane follows the newest line, how to say how many
// arrived while it did not, which keys do what, and what a screen reader is told. No React and no DOM here, so every branch can be
// script-checked.
import type { LoopState } from './followLoop'

// How close to the bottom (in pixels) still counts as "at the bottom": a fractional scroll position never makes the pane let go.
export const BOTTOM_SLACK_PX = 24

export interface ScrollMetrics {
  scrollTop: number
  clientHeight: number
  scrollHeight: number
}

export function isNearBottom(m: ScrollMetrics, slack: number = BOTTOM_SLACK_PX): boolean {
  return m.scrollHeight - (m.scrollTop + m.clientHeight) <= slack
}

// How many lines arrived since the person last saw the bottom (the lines have sequence numbers 0, 1, 2…; `seen` is the next number they
// had seen up to).
export function unseenLines(nextSeq: number, seenSeq: number): number {
  return Math.max(0, nextSeq - seenSeq)
}

export function jumpLabel(unseen: number): string {
  if (unseen <= 0) return 'Jump to latest'
  return `${unseen.toLocaleString('en-US')} new ${unseen === 1 ? 'line' : 'lines'} · jump to latest`
}

export type LiveAction = 'toggle' | 'bottom'

// F pauses and resumes; End or G jumps to the newest line. A key with Ctrl, Meta or Alt is the browser's or the system's, never ours.
export function keyToAction(key: string, modified = false): LiveAction | null {
  if (modified) return null
  if (key === 'f' || key === 'F') return 'toggle'
  if (key === 'End' || key === 'G') return 'bottom'
  return null
}

// What a screen reader is told when the state changes (not a line at a time: a log can write a hundred lines a second).
export function announce(loop: LoopState, paused: boolean): string {
  if (paused) return 'Paused.'
  switch (loop.kind) {
    case 'following':
      return 'Following the log.'
    case 'hidden':
      return ''
    case 'retrying':
      return loop.message
    case 'stopped':
      return loop.message
  }
}

// The words on the right of the status line next to what is held.
export function loopNote(loop: LoopState, paused: boolean): string | null {
  if (paused) return null
  switch (loop.kind) {
    case 'retrying':
      return loop.message
    case 'hidden':
      return 'waiting while this tab is hidden'
    case 'stopped':
      return loop.message
    case 'following':
      return null
  }
}
