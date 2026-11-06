// crawlEngine.ts (plan 190) — which Browser engine is selected, and what each engine needs to crawl a link's job list. Katana crawls with the
// link crawler's RULE, every other engine with the AI-written INSTRUCTIONS, and a link that needs a login is always read with its AI
// instructions. The screen judges "instructed" by this, never by what a link happens to store. Pure rules first (no React), so they can be
// script-checked; the fetch of the engine name at the end.
import { engineFor } from './engines/registry'
import type { CrawlEngine, EngineReading, JobDetailsAccess, LinkFacts, ListingNeeds } from './engines/types'

export type { CrawlEngine, EngineReading, JobDetailsAccess, ListingNeeds }

// Plan 192: what each engine decides lives in engines/<id>; these functions only ask it (nothing here names Rod or Katana).
export function listingNeeds(engine: EngineReading, requiresLogin: boolean): ListingNeeds {
  return engineFor(engine).listingNeeds(requiresLogin)
}

// Whether the AI writes this link's JOB-DETAIL instructions (plan 190). While the engine is not known, nothing is judged (true).
export function jobDetailIsWrittenByAi(engine: EngineReading, requiresLogin: boolean): boolean {
  return engineFor(engine).jobDetailsWrittenByAi(requiresLogin)
}

// Whether the automatic run of the AI for job-detail instructions may start: only once the engine IS known (never on the page's first
// render, before the answer arrives) and the AI writes this link's job details.
export function mayAutoWriteJobDetails(engine: EngineReading, requiresLogin: boolean): boolean {
  return engineFor(engine).mayAutoWriteJobDetails(requiresLogin)
}

// Whether a request for the AI to write the job details, left on the link, is stale under the engine and must be cleared.
export function clearsStaleJobDetailAiRequest(engine: EngineReading, requiresLogin: boolean): boolean {
  return engineFor(engine).clearsStaleJobDetailAiRequest(requiresLogin)
}

// Whether the link has what its job list needs under the engine in use.
export function hasListingFor(link: LinkFacts, engine: EngineReading): boolean {
  return engineFor(engine).hasListing(link)
}

const ENGINE_BASE = '/api/v1/tools/career/proxy/portal-links/crawl-engine'

// The selected engine, from the Career backend (which reads it from the Browser tool). Anything it does not say is 'unknown'.
export async function fetchCrawlEngine(signal?: AbortSignal): Promise<CrawlEngine> {
  const res = await fetch(ENGINE_BASE, { credentials: 'include', cache: 'no-store', signal })
  if (!res.ok) return 'unknown'
  const body = (await res.json()) as { engine?: string }
  return body.engine === 'katana' || body.engine === 'rod' ? body.engine : 'unknown'
}

// Plan 191: under Katana the Job details modal is the picker, which works on a real job page of the link, so it opens only for a link that
// has a crawled job; the engine decides (Rod, a login link and an engine not known yet stay open).
export function jobDetailsModalAccess(engine: EngineReading, link: LinkFacts): JobDetailsAccess {
  return engineFor(engine).jobDetailsAccess(link)
}

export const JOB_DETAILS_PILL = 'Crawl jobs first'

export function jobDetailsHint(access: JobDetailsAccess): string {
  switch (access) {
    case 'needs_jobs':
      return 'Crawl some jobs for this link first (Crawl now). Then you can pick the data of a job page here.'
    case 'needs_rule_and_jobs':
      return 'Make the job list rules first, then crawl some jobs for this link.'
    default:
      return ''
  }
}
