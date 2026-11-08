// instructionErrorRules.ts (plan 215) — whether the red "AI could not write the instructions" boxes of a link are shown. A fully instructed link shows
// none: a failure of an earlier run says nothing about a link whose instructions exist (the server also clears them; this covers what the page holds
// itself, such as the tab's own error from a run that has just ended). Pure, so it can be script-checked.
import type { InstructionState } from './instructionStats'

export function showsInstructionError(state: InstructionState, hasError: boolean): boolean {
  return hasError && state !== 'instructed'
}
