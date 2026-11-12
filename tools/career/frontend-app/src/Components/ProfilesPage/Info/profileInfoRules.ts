// Pure rules of the "How profiles work" infographic (plan 266): where the person is on the way to a first complete profile, worked out from the completeness
// each profile already carries (no extra request), and the order of the chapters. No React, so they can be script-checked.
import { chapterInView, infoEntryFor, nextStepOf, progressOf, type InfoEntry } from '../../../Shared/Infographic/infographicRules'
import type { JourneyStep } from '../../../Shared/Infographic/infographicTypes'
import { CHAPTERS, FIRST_STEPS, type Chapter, type ChapterId, type JourneyKey } from './profileInfoData'

export { chapterInView, infoEntryFor }
export type { InfoEntry, JourneyStep }

// The part of a profile the rules read: its completeness items (the key of each item and whether it is done) and the score they add up to.
export interface ProfileLike {
  completeness: { score: number; items: { key: string; done: boolean }[] }
}

// What the Profiles page knows for the journey: how many profiles there are and the completeness of the most complete one (null without a profile).
export interface JourneyFacts {
  profiles: number
  best: Record<string, boolean> | null
}

// The facts from the profiles the page loaded: the profile with the highest score is the one the path follows (the first of equals).
export function journeyFactsOf(profiles: ProfileLike[]): JourneyFacts {
  let best: ProfileLike | null = null
  for (const p of profiles) {
    if (best === null || p.completeness.score > best.completeness.score) best = p
  }
  if (best === null) return { profiles: 0, best: null }
  const done: Record<string, boolean> = {}
  for (const it of best.completeness.items) done[it.key] = it.done
  return { profiles: profiles.length, best: done }
}

// Whether each step of "Your first profile" is behind the person. A step counts as done when the thing it makes exists: a profile, a platform link, a
// target role, and a summary with skills and experience.
export function journeyState(facts: JourneyFacts): JourneyStep[] {
  const has = (id: string) => facts.best?.[id] === true
  const reached: Record<JourneyKey, boolean> = {
    profile: facts.profiles > 0,
    links: has('links'),
    role: has('persona'),
    content: has('summary') && has('skills') && has('experience'),
  }
  return FIRST_STEPS.map((s) => ({ id: s.id, done: s.track === null ? null : reached[s.track] }))
}

export function nextJourneyStep(facts: JourneyFacts): string | null {
  return nextStepOf(journeyState(facts))
}

export function journeyProgress(facts: JourneyFacts): { done: number; total: number } {
  return progressOf(journeyState(facts))
}

// The chapters in poster order.
export function chapterIds(): ChapterId[] {
  return CHAPTERS.map((c) => c.id)
}

export function chapterById(id: ChapterId): Chapter {
  const c = CHAPTERS.find((x) => x.id === id)
  if (!c) throw new Error(`unknown chapter ${id}`)
  return c
}

// The width of each part of the completeness bar, in percent of the bar (the weights add up to 100, so a weight is its own percentage).
export function partWidths(parts: { weight: number }[]): number[] {
  const total = parts.reduce((s, p) => s + p.weight, 0)
  return parts.map((p) => (total === 0 ? 0 : (p.weight / total) * 100))
}

// The score of a profile that has the given parts done, and the word for it (the backend's own bands).
export function scoreOf(doneIds: string[], parts: { id: string; weight: number }[]): number {
  return parts.filter((p) => doneIds.includes(p.id)).reduce((s, p) => s + p.weight, 0)
}

export function bandWord(score: number, bands: { word: string; from: number }[]): string {
  let word = bands[0].word
  for (const b of bands) if (score >= b.from) word = b.word
  return word
}
