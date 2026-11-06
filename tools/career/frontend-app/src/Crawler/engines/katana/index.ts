// The Katana engine on the screen (plan 192): the job list is crawled with a rule and the job page is read by picking its data (plans 190,
// 191). A link that needs a login has no rule and no picker: it keeps the AI instructions. It imports no other engine.
import { hasAiInstructions, hasRule, hasText } from '../linkFacts'
import type { InstructionEngine, LinkFacts } from '../types'

const byAi = (login: boolean) => login

export const katanaEngine: InstructionEngine = {
  id: 'katana',
  listingNeeds: (login) => (byAi(login) ? 'ai' : 'rule'),
  hasListing: (link: LinkFacts) => (byAi(Boolean(link.requiresLogin)) ? hasAiInstructions(link) : hasRule(link)),
  jobDetailsWrittenByAi: byAi,
  mayAutoWriteJobDetails: byAi,
  // A request left over from before (or raised by the crawl's event, which knows no engine) is stale under Katana.
  clearsStaleJobDetailAiRequest: (login) => !byAi(login),
  jobDetailsAccess(link) {
    if (byAi(Boolean(link.requiresLogin))) return 'open'
    // lastCrawledAt is the newest saved job of the link and is empty when the link has no job.
    if (hasText(link.lastCrawledAt)) return 'open'
    return hasRule(link) ? 'needs_jobs' : 'needs_rule_and_jobs'
  },
}
