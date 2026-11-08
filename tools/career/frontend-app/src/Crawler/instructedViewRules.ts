// instructedViewRules.ts — what a link's panel shows once the link is fully instructed (the "Instructed" state: job list and job details both have
// instructions). The attempt counter ("3 attempts · 1 succeeded · 2 failed", the red "no instructions after N attempts" pill) and the "Generate"
// buttons are about getting instructions written, so they are gone then; the tiles still open the editors. While the AI is writing instructions
// the progress button stays. Pure, so it can be script-checked.
import type { InstructionState } from './instructionStats'

export function showsAttemptStats(state: InstructionState): boolean {
  return state !== 'instructed'
}

// busy: the AI is writing this kind of instructions right now (its "Generating..." button, which opens the chat, must not vanish).
export function showsGenerate(state: InstructionState, busy: boolean): boolean {
  return state !== 'instructed' || busy
}
