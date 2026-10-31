// What the "latest AI run" card says about a run (step 112). Pure, so it can be script-checked.

import type { PersonaAiRun } from '../api'

export type RunTone = 'working' | 'ready' | 'failed'

export interface RunSummary {
  label: string
  tone: RunTone
  // One or two lines under the label.
  detail: string
}

// text, or the fallback when it is missing or blank.
const orElse = (text: string | undefined, fallback: string) => {
  const t = text?.trim() ?? ''
  return t === '' ? fallback : t
}

const names = (run: PersonaAiRun) => (run.proposal?.personas ?? []).map((p) => p.name)

export function summarizeRun(run: PersonaAiRun): RunSummary {
  if (run.status === 'generating') {
    return {
      label: 'The AI is working…',
      tone: 'working',
      detail: 'You can leave this page; the run is picked up again when you come back.',
    }
  }
  if (run.status === 'failed') {
    return {
      label: 'The run failed',
      tone: 'failed',
      detail: orElse(run.error, 'No reason was recorded. Open the chat to see what happened.'),
    }
  }
  const proposed = names(run)
  const created = run.personas
  const declined = run.declined
  // A run is decided when the server says so; one with personas created from it was decided too.
  if (run.decidedAt !== undefined || created.length > 0 || declined.length > 0) {
    const createdText = created.length === 1 ? '1 persona created' : `${created.length} personas created`
    const declinedText = `${declined.length} declined`
    const label =
      created.length > 0 && declined.length > 0
        ? `${created.length} created, ${declined.length} declined`
        : created.length > 0
          ? createdText
          : declined.length > 0
            ? declined.length === proposed.length
              ? 'All declined'
              : declinedText
            : 'Decided'
    const parts = [
      created.length > 0 ? created.map((p) => p.name).join(', ') : '',
      declined.length > 0 ? `Declined: ${declined.map((p) => p.name).join(', ')}` : '',
    ]
    return { label, tone: 'ready', detail: parts.filter((x) => x !== '').join('. ') }
  }
  if (proposed.length === 0) {
    return {
      label: 'The AI found nothing to propose',
      tone: 'ready',
      detail: orElse(run.proposal?.note, 'There was not enough data to base a persona on.'),
    }
  }
  return {
    label: proposed.length === 1 ? '1 persona proposed' : `${proposed.length} personas proposed`,
    tone: 'ready',
    detail: proposed.join(', '),
  }
}

// The run to show: the newest one (the list is newest first), or null with none.
export function latestRun(runs: PersonaAiRun[]): PersonaAiRun | null {
  return runs.length > 0 ? runs[0] : null
}

export function hasRunningRun(runs: PersonaAiRun[]): boolean {
  return runs.some((r) => r.status === 'generating')
}

// A ready run with personas proposed that the user has not decided on yet can be reviewed.
export function canReview(run: PersonaAiRun): boolean {
  return run.status === 'ready' && (run.proposal?.personas.length ?? 0) > 0 && run.decidedAt === undefined && run.personas.length === 0
}

// How many runs the history shows before "Show all".
export const HISTORY_LIMIT = 3

// The runs to show (newest first, as given): the first few, or all of them.
export function visibleRuns(runs: PersonaAiRun[], showAll: boolean): { shown: PersonaAiRun[]; hidden: number } {
  if (showAll || runs.length <= HISTORY_LIMIT) return { shown: runs, hidden: 0 }
  return { shown: runs.slice(0, HISTORY_LIMIT), hidden: runs.length - HISTORY_LIMIT }
}

// The conversation behind a persona's run, or null when the persona did not come from one or the
// run is not in the list (it was removed from the history).
export function chatOfRun(runs: PersonaAiRun[], runId: string | undefined): string | null {
  if (runId === undefined) return null
  return runs.find((r) => r.id === runId)?.conversationId ?? null
}

// A run can only be removed from the history once it is no longer working.
export function canRemove(run: PersonaAiRun): boolean {
  return run.status !== 'generating'
}

// The time a run was created. The server stores UTC as "YYYY-MM-DD HH:MM:SS"; shown in local time,
// or as given when it is not a date.
export function formatRunTime(createdAt: string): string {
  const date = new Date(/^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$/.test(createdAt) ? `${createdAt.replace(' ', 'T')}Z` : createdAt)
  return Number.isNaN(date.getTime()) ? createdAt : date.toLocaleString([], { dateStyle: 'medium', timeStyle: 'short' })
}
