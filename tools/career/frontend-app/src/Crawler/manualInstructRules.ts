// manualInstructRules.ts — when a link that is not instructed offers "Manually" beside "Generate", so it is plain that the instructions can also be written
// by hand. It opens the editor the tile already opens (view/edit), nothing new is saved by showing it. Pure, so it can be script-checked.

const stored = (text: string | null | undefined) => (text ?? '').trim() !== ''

// The job list: not instructed yet (nothing is stored for the engine in use: `hasListing`), and the AI is not writing them right now.
export function showsManualListing(hasListing: boolean, aiBusy: boolean): boolean {
  return !hasListing && !aiBusy
}

// The job details: no instructions stored, the editor is reachable (not blocked behind "crawl jobs first"), and the AI is not writing them right now.
export function showsManualJobDetails(jobDetailInstructions: string | null | undefined, blocked: boolean, aiBusy: boolean): boolean {
  return !stored(jobDetailInstructions) && !blocked && !aiBusy
}
