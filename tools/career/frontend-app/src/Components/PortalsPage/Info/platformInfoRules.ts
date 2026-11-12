// Pure rules of the "How platforms work" infographic (plan 262): the order of its chapters, where the person is on the way to a first crawl,
// and the one decision about what the Platforms page shows (the short teaser, or the button and the card). No React, so they can be script-checked.
import { CHAPTERS, FIRST_STEPS, type Chapter, type ChapterId, type JourneyKey } from './platformInfoData'

// What the Platforms page already knows (summarizePortals): no extra request.
export interface JourneyFacts {
  portals: number
  links: number
  // Links that have what the engine in use needs to crawl them.
  withInstructions: number
  // Links that never saved a job.
  neverCrawled: number
}

export interface JourneyStep {
  id: string
  // null: the page cannot tell (choosing the AI, looking at the jobs); drawn without a done mark.
  done: boolean | null
}

// Whether each step of "Your first platform" is behind the person. A step counts as done when the thing it makes exists: a platform, a link,
// a link with instructions, a link that saved a job.
export function journeyState(facts: JourneyFacts): JourneyStep[] {
  const reached: Record<JourneyKey, boolean> = {
    platform: facts.portals > 0,
    link: facts.links > 0,
    instructions: facts.withInstructions > 0,
    crawled: facts.links - facts.neverCrawled > 0,
  }
  return FIRST_STEPS.map((s) => ({ id: s.id, done: s.track === null ? null : reached[s.track] }))
}

// The first step the person has still to do (one the page can tell about), or null when every one it can tell about is done.
export function nextJourneyStep(facts: JourneyFacts): string | null {
  const step = journeyState(facts).find((s) => s.done === false)
  return step ? step.id : null
}

// How many of the steps the page can tell about are done, and how many that is in all (for "3 of 4 done").
export function journeyProgress(facts: JourneyFacts): { done: number; total: number } {
  const known = journeyState(facts).filter((s) => s.done !== null)
  return { done: known.filter((s) => s.done).length, total: known.length }
}

// What the page shows about the infographic. `loaded` is false until the list of platforms was read: nothing is shown before, so the teaser
// never flashes for a person who has platforms.
export type InfoEntry = 'none' | 'teaser' | 'button'

export function infoEntryFor(loaded: boolean, platformCount: number): InfoEntry {
  if (!loaded) return 'none'
  return platformCount === 0 ? 'teaser' : 'button'
}

// The chapters in poster order, and the position of one (the chapter bar marks the one in view).
export function chapterIds(): ChapterId[] {
  return CHAPTERS.map((c) => c.id)
}

export function chapterById(id: ChapterId): Chapter {
  const c = CHAPTERS.find((x) => x.id === id)
  if (!c) throw new Error(`unknown chapter ${id}`)
  return c
}

// The chapter in view: the last one whose top is at or above `line` (a y position), given the tops in poster order; the first when none is.
export function chapterInView(tops: number[], line: number): number {
  let current = 0
  for (let i = 0; i < tops.length; i++) {
    if (tops[i] <= line) current = i
  }
  return current
}
