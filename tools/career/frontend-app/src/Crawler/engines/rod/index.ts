// The Rod engine on the screen (plan 192): the AI writes the selectors of the job list and of the job page. The existing Rod modals and
// prompt builders stay where they are and are not edited; this object only holds the judgements. It imports no other engine.
import { hasAiInstructions } from '../linkFacts'
import type { InstructionEngine } from '../types'

export const rodEngine: InstructionEngine = {
  id: 'rod',
  listingNeeds: () => 'ai',
  hasListing: hasAiInstructions,
  jobDetailsWrittenByAi: () => true,
  mayAutoWriteJobDetails: () => true,
  aiTriesListing: () => true,
  clearsStaleJobDetailAiRequest: () => false,
  // Rod's job-detail modal is a text box a person can type into: it never waits for jobs.
  jobDetailsAccess: () => 'open',
}
