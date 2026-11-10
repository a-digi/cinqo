import type { JobApplication } from '../../api'
import { plural } from '../CompaniesPage/companyStats'
import { agoText, protocolChip } from '../JobBoard/protocolChipRules'

// The rules of the overview of applications (plan/ai/tools/career/step-176-application-protocol.md, step 5).

export type ApplicationSort = 'recent' | 'oldest' | 'waiting' | 'title'

export const SORTS: { value: ApplicationSort; label: string }[] = [
  { value: 'recent', label: 'Latest activity' },
  { value: 'oldest', label: 'Oldest activity' },
  { value: 'waiting', label: 'Waiting for a reply' },
  { value: 'title', label: 'Job title' },
]

// The time of the last activity as a number to compare; an application without one counts as "never" (null).
function at(a: JobApplication): number | null {
  return a.activityAt === undefined ? null : new Date(a.activityAt).getTime()
}

// Newest first, or oldest first; the ones without activity come last either way; ties by title, then id.
function byActivity(newestFirst: boolean) {
  return (a: JobApplication, b: JobApplication): number => {
    const x = at(a)
    const y = at(b)
    if (x !== y) {
      if (x === null) return 1
      if (y === null) return -1
      return newestFirst ? y - x : x - y
    }
    return byTitle(a, b)
  }
}

function byTitle(a: JobApplication, b: JobApplication): number {
  const t = a.title.toLowerCase().localeCompare(b.title.toLowerCase())
  return t !== 0 ? t : a.jobId.localeCompare(b.jobId)
}

// The applications in the chosen order, as a new array. "Waiting for a reply" puts the ones that wait first, the longest wait first, and
// the rest after them, newest first.
export function sortApplications(list: readonly JobApplication[], sort: ApplicationSort): JobApplication[] {
  const copy = [...list]
  switch (sort) {
    case 'recent':
      return copy.sort(byActivity(true))
    case 'oldest':
      return copy.sort(byActivity(false))
    case 'title':
      return copy.sort(byTitle)
    case 'waiting':
      return copy.sort((a, b) => {
        if (a.waitingForReply !== b.waitingForReply) return a.waitingForReply ? -1 : 1
        return a.waitingForReply ? byActivity(false)(a, b) : byActivity(true)(a, b)
      })
  }
}

// How many of the applications wait for a reply.
export function waitingCount(list: readonly JobApplication[]): number {
  return list.filter((a) => a.waitingForReply).length
}

// The page's totals, now that it has no header (plan 237): "5 applications · 2 waiting", or "1 application" when none waits.
export function totalsText(total: number, waiting: number): string {
  const all = plural(total, 'application', 'applications')
  return waiting > 0 ? `${all} · ${String(waiting)} waiting` : all
}

// What the row says about the protocol: "3 entries · You wrote 2 days ago · 1 planned", or "No entries yet" when there is none.
export function protocolText(a: JobApplication, now: Date): string {
  if (a.protocol === undefined) return 'No entries yet'
  const chip = protocolChip(a.protocol, now)
  return [chip.count, chip.last, chip.planned].filter((part) => part !== '').join(' · ')
}

// When something last happened on the application: "today", "3 days ago", the date from 30 days on; "nothing yet" for none.
export function activityText(a: JobApplication, now: Date): string {
  return a.activityAt === undefined ? 'nothing yet' : agoText(a.activityAt, now)
}
