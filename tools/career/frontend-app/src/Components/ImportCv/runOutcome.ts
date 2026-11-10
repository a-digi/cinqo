// Pure rules for a declined import (plan/ai/tools/career/step-242-restart-cv-import-inside-the-import-page.md, step 2). Declining a review is not a new
// status of the run: it is a save summary with a flag, written through the route that already records the outcome of a review, so the server, the
// database and every older page keep working. No React, so they can be script-checked.

// The save summary of a declined import: nothing was inserted (every list empty) and the flag says it was a decision, not an unfinished review.
export interface DeclinedSummary {
  declined: true
  skills: []
  experience: []
  education: []
}

export function buildDeclinedSummary(): DeclinedSummary {
  return { declined: true, skills: [], experience: [], education: [] }
}

// Whether a stored save summary says the import was declined: an object whose `declined` is exactly true. Anything else (nothing, null, text, a normal
// summary of inserted rows, a flag that is not `true`) is not.
export function isDeclinedSummary(summary: unknown): boolean {
  return typeof summary === 'object' && summary !== null && !Array.isArray(summary) && (summary as { declined?: unknown }).declined === true
}

// A run is declined when it is ready (it has its proposal) and its summary says so. A failed or still-running run cannot be declined.
export function isDeclinedRun(run: { status: string; saveSummary?: unknown }): boolean {
  return run.status === 'ready' && isDeclinedSummary(run.saveSummary)
}

// What a person is told on a declined import.
export const DECLINED_MESSAGE = 'You declined this import. Nothing from it was saved.'
