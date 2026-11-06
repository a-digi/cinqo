// engines/types.ts (plan 192) — the interface of an instruction engine on the screen, and the neutral types every engine and the shared
// screen code use. Rod (the AI writes CSS selectors) and Katana (a link rule and picked job-page data) are two engines behind it, each in a
// folder of its own (engines/rod, engines/katana). An engine imports nothing but this file and linkFacts.ts; only registry.ts names them.
import type { PortalLink } from '../../api'

export type CrawlEngine = 'katana' | 'rod' | 'unknown'
// null: not asked yet.
export type EngineReading = CrawlEngine | null
// What the job list of a link needs: its rule, its AI instructions, or either (the engine is not known, so nothing is judged by it).
export type ListingNeeds = 'rule' | 'ai' | 'either'
// Plan 191: whether the Job details modal may open. 'needs_jobs' = the job list is instructed but no job was crawled yet;
// 'needs_rule_and_jobs' = there is no rule either, so no job can exist.
export type JobDetailsAccess = 'open' | 'needs_jobs' | 'needs_rule_and_jobs'

// The fields of a link the engines judge by.
export type LinkFacts = Partial<Pick<PortalLink, 'crawlInstructions' | 'linkRules' | 'lastCrawledAt'> & { requiresLogin: boolean }>

export interface InstructionEngine {
  // The Browser engine name this engine serves ('rod', 'katana'); 'unknown' is the engine that judges nothing.
  readonly id: string
  // What the job list of a link needs under this engine (a link that needs a login is always read with its AI instructions).
  listingNeeds(requiresLogin: boolean): ListingNeeds
  // Whether the link has what its job list needs.
  hasListing(link: LinkFacts): boolean
  // Whether the AI writes this link's JOB-DETAIL instructions (otherwise they are picked).
  jobDetailsWrittenByAi(requiresLogin: boolean): boolean
  // Whether the automatic run of the AI for job-detail instructions may start for this link.
  mayAutoWriteJobDetails(requiresLogin: boolean): boolean
  // Whether a request for the AI to write the job details, left on the link, is stale under this engine and must be cleared.
  clearsStaleJobDetailAiRequest(requiresLogin: boolean): boolean
  // Whether the Job details modal may open (plan 191).
  jobDetailsAccess(link: LinkFacts): JobDetailsAccess
}
