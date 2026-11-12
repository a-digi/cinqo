// Pure rules of the "How platforms work" infographic (plan 262): the order of its chapters, where the person is on the way to a first crawl,
// and the one decision about what the Platforms page shows (the short teaser, or the button and the card). No React, so they can be script-checked.
import { chapterInView, infoEntryFor, nextStepOf, progressOf, type InfoEntry } from '../../../Shared/Infographic/infographicRules'
import type { JourneyStep } from '../../../Shared/Infographic/infographicTypes'
import { CHAPTERS, FIRST_STEPS, type Chapter, type ChapterId, type JourneyKey } from './platformInfoData'

// The rules every infographic shares live in Shared/Infographic; they are named here too, as the platform code and its tests always used them.
export { chapterInView, infoEntryFor }
export type { InfoEntry, JourneyStep }

// What the Platforms page already knows (summarizePortals): no extra request.
export interface JourneyFacts {
  portals: number
  links: number
  // Links that have what the engine in use needs to crawl them.
  withInstructions: number
  // Links that never saved a job.
  neverCrawled: number
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
  return nextStepOf(journeyState(facts))
}

// How many of the steps the page can tell about are done, and how many that is in all (for "3 of 4 done").
export function journeyProgress(facts: JourneyFacts): { done: number; total: number } {
  return progressOf(journeyState(facts))
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
