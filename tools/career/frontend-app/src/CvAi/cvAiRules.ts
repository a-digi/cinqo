// Pure rules for the "create a CV with AI" runs (step 115), kept apart from React so they can be
// script-checked.
import type { CvAiRun } from '../api'

// An AI run is already working on the persona (the server refuses a second one; the button waits).
export function hasRunningCvRun(runs: CvAiRun[]): boolean {
  return runs.some((r) => r.status === 'generating')
}

// What the server says when the AI finished without saving a CV, shown to the user.
export const NO_CV_SAVED_MESSAGE = 'The AI did not create a CV. Open the chat to see what happened, or try again.'

// The run to tell the user about when it failed: only the newest run counts (the list is newest
// first), so an old failure does not nag once a later run worked or is working, and a failure the
// user dismissed stays dismissed (the run itself stays in the history, with its chat).
export function failedRunToShow(runs: CvAiRun[], dismissed: ReadonlySet<string> = new Set()): CvAiRun | null {
  return runs.length > 0 && runs[0].status === 'failed' && !dismissed.has(runs[0].id) ? runs[0] : null
}

// The chat of the run that made each CV (a CV id maps to its run's conversation id); a CV made by hand,
// or whose run was removed, is not in it. The first (newest) run wins when two name the same CV.
export function chatsByCv(runs: Pick<CvAiRun, 'cvDocumentId' | 'conversationId'>[]): Map<string, string> {
  const out = new Map<string, string>()
  for (const r of runs) {
    if (r.cvDocumentId !== undefined && !out.has(r.cvDocumentId)) out.set(r.cvDocumentId, r.conversationId)
  }
  return out
}

// The CV a finished run created that this page watched work: the run ids seen generating are given.
// Null while nothing new has finished.
export function newlyCreatedCv(runs: CvAiRun[], watched: ReadonlySet<string>): string | null {
  const done = runs.find((r) => r.status === 'ready' && r.cvDocumentId !== undefined && watched.has(r.id))
  return done?.cvDocumentId ?? null
}

export interface RunBadge {
  label: string
  tone: 'working' | 'ready' | 'failed'
}

// How the history names a run.
export function runBadge(run: CvAiRun): RunBadge {
  if (run.status === 'generating') return { label: 'Working', tone: 'working' }
  if (run.status === 'failed') return { label: 'Failed', tone: 'failed' }
  return { label: run.cvDocumentId === undefined ? 'CV created (since deleted)' : 'CV created', tone: 'ready' }
}

// How many runs the history shows before "Show all".
export const CV_HISTORY_LIMIT = 3

export function visibleCvRuns<T>(runs: T[], showAll: boolean): { shown: T[]; hidden: number } {
  if (showAll || runs.length <= CV_HISTORY_LIMIT) return { shown: runs, hidden: 0 }
  return { shown: runs.slice(0, CV_HISTORY_LIMIT), hidden: runs.length - CV_HISTORY_LIMIT }
}

// Opens a run's chat in the chat window at the bottom of the app, where every conversation is shown.
export function openRunChat(run: Pick<CvAiRun, 'conversationId'>): void {
  window.__cinqoToolBridge.openConversation(run.conversationId)
}
