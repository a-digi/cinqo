// Pure rules for what a running import shows (plan 209): the checklist of what is stored so far, how long it has been working, and whether the
// page that was driving it is gone. No React, so they can be script-checked.
import type { CVImportProgress } from '../../api'

export type ActivityState = 'done' | 'working' | 'todo'

export interface ActivityItem {
  key: 'profile' | 'persona' | 'details' | 'skills' | 'jobs' | 'education'
  label: string
  state: ActivityState
  // "30 stored" for a list; empty for a single section.
  detail: string
}

// The checklist in the order the import does its parts. The first part that is not stored yet is the one being worked on; a list that has
// something in it is done so far (jobs come in one at a time, so "3 stored" is still growing).
export function activityItems(progress: CVImportProgress | undefined): ActivityItem[] {
  const p: CVImportProgress = progress ?? { profile: false, persona: false, personalDetails: false, skills: 0, experience: 0, education: 0 }
  const parts: { key: ActivityItem['key']; label: string; done: boolean; count?: number }[] = [
    { key: 'profile', label: 'Profile', done: p.profile },
    { key: 'persona', label: 'Target role', done: p.persona },
    { key: 'details', label: 'Personal details', done: p.personalDetails },
    { key: 'skills', label: 'Skills', done: p.skills > 0, count: p.skills },
    { key: 'jobs', label: 'Work experience', done: p.experience > 0, count: p.experience },
    { key: 'education', label: 'Education', done: p.education > 0, count: p.education },
  ]
  const firstOpen = parts.findIndex((part) => !part.done)
  return parts.map((part, i) => ({
    key: part.key,
    label: part.label,
    state: part.done ? 'done' : i === firstOpen ? 'working' : 'todo',
    detail: part.count !== undefined && part.count > 0 ? `${String(part.count)} stored` : '',
  }))
}

// A time stored by the Career backend ("2026-11-08 16:21:10", UTC, no zone) as a Date; null when it cannot be read.
export function parseStoredTime(text: string | undefined): Date | null {
  if (!text) return null
  const d = new Date(`${text.trim().replace(' ', 'T')}Z`)
  return Number.isNaN(d.getTime()) ? null : d
}

// "0:07", "1:05", "12:30": how long a run has been working. Never negative (a clock a little behind the server's).
export function elapsedLabel(createdAt: string | undefined, now: Date): string {
  const started = parseStoredTime(createdAt)
  if (!started) return ''
  const seconds = Math.max(0, Math.floor((now.getTime() - started.getTime()) / 1000))
  return `${String(Math.floor(seconds / 60))}:${String(seconds % 60).padStart(2, '0')}`
}

// A run driven in code lives in the page that started it: when that page is closed nobody carries it on. It is taken for abandoned when this page
// is not driving it and nothing was stored (or asked) for a long while. A question to a slow model takes a minute at most, so three is safe.
export const ABANDONED_AFTER_MS = 3 * 60 * 1000

export function isAbandonedCodeRun(
  run: { status: string; driver?: string; createdAt: string; updatedAt?: string },
  now: Date,
  drivenHere: boolean,
): boolean {
  if (run.status !== 'generating' || run.driver !== 'code' || drivenHere) return false
  const last = parseStoredTime(run.updatedAt) ?? parseStoredTime(run.createdAt)
  return last !== null && now.getTime() - last.getTime() > ABANDONED_AFTER_MS
}

export const ABANDONED_MESSAGE = 'The page that ran this import was closed before it finished. Process the CV again.'
