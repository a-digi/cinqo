// Pure rules for the Import history list (step 155): what a run's status looks like and what can be done with it. A run is
// "generating" while the AI analyses the CV, "ready" once its proposal is there (every run from before step 155 is ready) and
// "failed" when the analysis did not produce one. No React, so they can be script-checked.
export type RunStatus = 'generating' | 'ready' | 'failed'

export interface RunLike {
  status: RunStatus
  error?: string
}

export type BadgeTone = 'amber' | 'red'

// The badge of a run in the list, or null for a ready run (it needs none: it has its proposal and its View button).
export function runBadge(run: RunLike): { label: string; tone: BadgeTone; title: string } | null {
  switch (run.status) {
    case 'generating':
      return { label: 'Analysing…', tone: 'amber', title: 'The AI is still analysing this CV, or the page was closed before it finished.' }
    case 'failed':
      return { label: 'Failed', tone: 'red', title: run.error && run.error.trim() !== '' ? run.error : 'The analysis did not finish.' }
    default:
      return null
  }
}

// Only a ready run has a proposal to view.
export function canViewRun(run: RunLike): boolean {
  return run.status === 'ready'
}

// What a person is told when a run could not be finished as ready: the server's message when there is one.
export function failureReason(err: unknown): string {
  const text = err instanceof Error ? err.message : String(err)
  return text.trim() === '' ? 'The analysis did not finish.' : text.trim()
}

// The text the page shows when the AI's turn ended without a proposal (the server refused to end the run as ready).
export const NO_PROPOSAL_MESSAGE = 'The AI finished without handing over a proposal. Start over, or process the CV again.'
