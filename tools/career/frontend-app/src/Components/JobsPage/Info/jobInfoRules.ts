// The pure rules of the Jobs infographic (plan 269): which view the Jobs page shows, which entry points are offered, and how far the first steps are, from what the page already knows. No React.
import { nextStepOf, progressOf } from '../../../Shared/Infographic/infographicRules'
import type { JourneyStep } from '../../../Shared/Infographic/infographicTypes'
import type { SectionId } from './jobInfoData'
import { SECTIONS } from './jobInfoData'

export type { JourneyStep }

// What the page's statistics are doing, as useJobStats reports it.
export type StatsState = 'loading' | 'failed' | 'ready'

// Which page the Jobs page is: 'loading' and 'failed' keep the usual page (never a flash of the summary for a person who has jobs, and a failed read says nothing about the jobs);
// with the statistics read and no job at all it is only the summary (the user's rule of 2026-11-13: no table, no cards, no filters); otherwise the usual list.
export type JobsView = 'usual' | 'summary' | 'list'

export function jobsPageView(state: StatsState, total: number): JobsView {
  if (state !== 'ready') return 'usual'
  return total === 0 ? 'summary' : 'list'
}

// Whether the page shows the full list (the table, the cards, the filters, the right column): in the summary view it does not; while loading or after a failure it does, as before.
export function showsList(view: JobsView): boolean {
  return view !== 'summary'
}

// Which entry point to the infographic the page offers: the summary is itself the entry; with jobs a button in the button row; while the numbers are not known, none.
export type JobInfoEntry = 'none' | 'summary' | 'button'
export function jobInfoEntry(view: JobsView, state: StatsState): JobInfoEntry {
  if (view === 'summary') return 'summary'
  return state === 'ready' ? 'button' : 'none'
}

// What the page knows of the person's progress: how many platforms there are (null: the list did not load) and the statistics' counts (null: not read).
export interface JobFacts {
  platforms: number | null
  stats: { total: number; matched: number } | null
}

// The four first steps. A step the page cannot tell about is `null` (drawn without a done mark): the links of a platform are not loaded here, but once jobs have arrived they obviously exist.
export function journeyState(f: JobFacts): JourneyStep[] {
  const total = f.stats?.total ?? null
  return [
    { id: 'platform', done: f.platforms === null ? null : f.platforms > 0 || (total !== null && total > 0) },
    { id: 'links', done: total !== null && total > 0 ? true : null },
    { id: 'jobs', done: total === null ? null : total > 0 },
    { id: 'match', done: f.stats === null ? null : f.stats.matched > 0 },
  ]
}

export function journeyProgress(f: JobFacts): { done: number; total: number } {
  return progressOf(journeyState(f))
}

export function nextJourneyStep(f: JobFacts): string | null {
  return nextStepOf(journeyState(f))
}

// The summary names the next thing to do: with no platform, add one; with a platform and no job yet, give it its links and let it crawl (or add a job). A platform list that did not load
// says the first.
export function summaryNext(platforms: number | null): 'platform' | 'crawl' {
  return platforms !== null && platforms > 0 ? 'crawl' : 'platform'
}

// The sections of the local bar and the order of the bands.
export function sectionIds(): SectionId[] {
  return SECTIONS.map((s) => s.id)
}
