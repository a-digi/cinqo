// engines/types.ts (plan 192) — the interface of an instruction engine on the screen, and the neutral types every engine and the shared
// screen code use. Rod (the AI writes CSS selectors) and Katana (a link rule and picked job-page data) are two engines behind it, each in a
// folder of its own (engines/rod, engines/katana). An engine imports nothing but this file and linkFacts.ts; only registry.ts names them.
import type { ComponentType } from 'react'
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
export type LinkFacts = Partial<
  Pick<PortalLink, 'crawlInstructions' | 'linkRules' | 'lastCrawledAt'> & { requiresLogin: boolean; loginSession: string }
>

// What the engine's own "start over" control is given by the link card.
export interface StartOverProps {
  link: PortalLink
  // Why the control is disabled (a run is going), or null.
  blockedReason: string | null
  onReload: () => void
}

// How an engine's AI writes the JOB LIST of a link (the Generate button): the tools its hidden conversation may use, the tool-call budget
// and the prompt. An engine without one (Rod) is run by the existing Generate code unchanged.
export interface ListingGenerate {
  tools: readonly string[]
  toolIterationLimit: number
  buildMessage: (link: Pick<PortalLink, 'id' | 'title' | 'url'>) => string
}

// What the engine's own progress view is given by the link card.
export interface ProgressProps {
  link: PortalLink
  onReload: () => void
}

export interface InstructionEngine {
  // The Browser engine name this engine serves ('rod', 'katana'); 'unknown' is the engine that judges nothing.
  readonly id: string
  // Whether this engine reads a link that needs a login with a login kept in the Browser tool (plan 195, step 4), so no login window is needed.
  // Absent = no (Rod keeps the login window).
  readonly usesKeptLogin?: boolean
  // What the job list of a link needs under this engine (a link that needs a login and has no kept login is always read with its AI instructions).
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
  // Whether the AI's attempts at the job list are counted and shown (the attempt pills) under this engine.
  aiTriesListing(requiresLogin: boolean): boolean
  // The engine's own recipe for the Generate button on the job list, for a link that needs no login (Katana only).
  readonly listingGenerate?: ListingGenerate
  // The engine's own recipe for the Generate button on the job details, for a link that needs no login (Katana only).
  readonly jobDetailGenerate?: ListingGenerate
  // The engine's own view of how far the instructing of a link has come (Katana: job list, jobs, job details), for a link that needs no login.
  readonly progressView?: ComponentType<ProgressProps>
  // The engine's own control to delete what it made for a link and begin again (Katana only; Rod has "Start fresh" in the card as before).
  readonly startOverControl?: ComponentType<StartOverProps>
}
