// Pure rules every "How it works" infographic shares (plan 262, shared by plan 266): which chapter is in view, what a page shows about the infographic,
// and how far along the journey a person is. No React, so they can be script-checked.
import type { JourneyStep } from './infographicTypes'

// What a page shows about the infographic. `loaded` is false until the list was read: nothing is shown before, so the teaser never flashes for a person
// who has entries.
export type InfoEntry = 'none' | 'teaser' | 'button'

export function infoEntryFor(loaded: boolean, entryCount: number): InfoEntry {
  if (!loaded) return 'none'
  return entryCount === 0 ? 'teaser' : 'button'
}

// The chapter in view: the last one whose top is at or above `line` (a y position), given the tops in poster order; the first when none is.
export function chapterInView(tops: number[], line: number): number {
  let current = 0
  for (let i = 0; i < tops.length; i++) {
    if (tops[i] <= line) current = i
  }
  return current
}

// The first step the person has still to do (one the page can tell about), or null when every one it can tell about is done.
export function nextStepOf(steps: JourneyStep[]): string | null {
  const step = steps.find((s) => s.done === false)
  return step ? step.id : null
}

// How many of the steps the page can tell about are done, and how many that is in all (for "3 of 4 done").
export function progressOf(steps: JourneyStep[]): { done: number; total: number } {
  const known = steps.filter((s) => s.done !== null)
  return { done: known.filter((s) => s.done).length, total: known.length }
}
