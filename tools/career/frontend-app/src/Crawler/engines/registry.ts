// The ONE file that names the instruction engines on the screen (plan 192). Removing an engine means deleting its folder and its line here.
// An engine that is not registered (removed, or a name this build does not know) and a reading that has not arrived yet are served by the
// engine that judges nothing.
import { katanaEngine } from './katana'
import { rodEngine } from './rod'
import type { EngineReading, InstructionEngine } from './types'
import { hasAiInstructions, hasRule } from './linkFacts'

export const instructionEngines: readonly InstructionEngine[] = [rodEngine, katanaEngine]

// Nothing is judged by an engine: either kind of instructions counts, nothing is cleared, nothing starts by itself, nothing is blocked.
export const unreadEngine: InstructionEngine = {
  id: 'unknown',
  listingNeeds: (login) => (login ? 'ai' : 'either'),
  hasListing: (link) => (link.requiresLogin ? hasAiInstructions(link) : hasRule(link) || hasAiInstructions(link)),
  jobDetailsWrittenByAi: () => true,
  mayAutoWriteJobDetails: () => false,
  clearsStaleJobDetailAiRequest: () => false,
  jobDetailsAccess: () => 'open',
}

export function engineFor(reading: EngineReading): InstructionEngine {
  return instructionEngines.find((e) => e.id === reading) ?? unreadEngine
}
