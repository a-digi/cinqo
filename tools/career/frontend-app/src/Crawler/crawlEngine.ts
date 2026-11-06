// crawlEngine.ts (plan 190) — which Browser engine is selected, and what each engine needs to crawl a link's job list. Katana crawls with the
// link crawler's RULE, every other engine with the AI-written INSTRUCTIONS, and a link that needs a login is always read with its AI
// instructions. The screen judges "instructed" by this, never by what a link happens to store. Pure rules first (no React), so they can be
// script-checked; the fetch of the engine name at the end.
import type { PortalLink } from '../api'

export type CrawlEngine = 'katana' | 'rod' | 'unknown'
// null: not asked yet.
export type EngineReading = CrawlEngine | null
// What the job list of a link needs: its rule, its AI instructions, or either (the engine is not known, so nothing is judged by it).
export type ListingNeeds = 'rule' | 'ai' | 'either'

export function listingNeeds(engine: EngineReading, requiresLogin: boolean): ListingNeeds {
  if (requiresLogin) return 'ai'
  if (engine === 'katana') return 'rule'
  if (engine === 'rod') return 'ai'
  return 'either'
}

// Whether the AI writes this link's JOB-DETAIL instructions (plan 190): not under Katana, where the job page is read by picking its data; a
// link that needs a login has no picker and keeps the AI. While the engine is not known, nothing is judged (true).
export function jobDetailIsWrittenByAi(engine: EngineReading, requiresLogin: boolean): boolean {
  return listingNeeds(engine, requiresLogin) !== 'rule'
}

// Whether the automatic run of the AI for job-detail instructions may start: only once the engine IS known (never on the page's first
// render, before the answer arrives) and the AI writes this link's job details.
export function mayAutoWriteJobDetails(engine: EngineReading, requiresLogin: boolean): boolean {
  return engine !== null && engine !== 'unknown' && jobDetailIsWrittenByAi(engine, requiresLogin)
}

type ListingFields = Pick<PortalLink, 'crawlInstructions' | 'linkRules'> & { requiresLogin?: boolean }

const hasText = (text: string | null | undefined) => (text ?? '').trim() !== ''

// Whether the link has what its job list needs under the engine in use.
export function hasListingFor(link: Partial<ListingFields>, engine: EngineReading): boolean {
  const rule = (link.linkRules ?? null) !== null
  const ai = hasText(link.crawlInstructions)
  switch (listingNeeds(engine, Boolean(link.requiresLogin))) {
    case 'rule':
      return rule
    case 'ai':
      return ai
    default:
      return rule || ai
  }
}

const ENGINE_BASE = '/api/v1/tools/career/proxy/portal-links/crawl-engine'

// The selected engine, from the Career backend (which reads it from the Browser tool). Anything it does not say is 'unknown'.
export async function fetchCrawlEngine(signal?: AbortSignal): Promise<CrawlEngine> {
  const res = await fetch(ENGINE_BASE, { credentials: 'include', cache: 'no-store', signal })
  if (!res.ok) return 'unknown'
  const body = (await res.json()) as { engine?: string }
  return body.engine === 'katana' || body.engine === 'rod' ? body.engine : 'unknown'
}
