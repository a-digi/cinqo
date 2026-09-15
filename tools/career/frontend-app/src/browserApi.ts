// browserApi.ts — the deterministic ("Crawl now") crawl path (step
// 27): plain proxy calls to browser's own already-existing, already-
// deterministic /crawl and /crawl-paginated routes
// (/api/v1/tools/browser/proxy/...), plus this tool's own two new
// proxy routes that turn stored crawl_instructions into the request
// those need and turn their result into saved jobs
// (/api/v1/tools/career/proxy/portal-links/...). No AI, no
// conversation, no platform/API key anywhere in this file. See
// plan/ai/tools/career/step-27-ai-free-manual-crawl.md.
//
// Deliberately separate from coreApi.ts (which calls cinqo's CORE,
// non-proxy endpoints for the "Crawl with AI" path) — this file only
// ever calls proxy routes, the same PROXY_BASE convention api.ts
// already uses for this tool's own backend, just against a
// *different* tool's proxy for the browser-tool calls.

const CAREER_PROXY_BASE = '/api/v1/tools/career/proxy'
const BROWSER_PROXY_BASE = '/api/v1/tools/browser/proxy'

// Both career's own handlers (writeJSON({...})) and core's own proxy-
// level failures (response.ErrorResponse's {error:true,message:"..."})
// can produce the error responses this sees — parsed for a clean
// `.message` either way, rather than surfacing the raw JSON body text
// verbatim to the user.
async function proxyJsonOrThrow<T>(res: Response, action: string): Promise<T> {
  if (!res.ok) {
    const text = await res.text().catch(() => '')
    let message = text
    try {
      const parsed = JSON.parse(text) as { message?: unknown }
      if (parsed && typeof parsed.message === 'string') message = parsed.message
    } catch {
      // Not JSON (e.g. a plain-text 404/502 from a proxied backend) — use the raw text as-is.
    }
    throw new Error(message || `failed to ${action} (${res.status})`)
  }
  return res.json() as Promise<T>
}

export interface CrawlRequestField {
  label: string
  selector: string
  attribute?: string
  multiple?: boolean
}

export interface CrawlRequest {
  // Optional (step 30) — a CSS selector for each repeating item's own
  // wrapping element. Purely passed through by this file: career's own
  // backend both produces this (crawl-request) and consumes crawlPaginated's
  // resulting `items` per page (ingest-crawl-results) — the frontend
  // never inspects or transforms it.
  container?: string
  fields: CrawlRequestField[]
  nextSelector: string
  requestedMaxPages: number
  effectiveMaxPages: number
}

// GET career/portal-links/crawl-request?id=... — the link's own
// stored crawl_instructions, already parsed into the exact JSON shape
// /crawl-paginated below expects.
export async function fetchCrawlRequest(portalLinkId: string): Promise<CrawlRequest> {
  const res = await fetch(`${CAREER_PROXY_BASE}/portal-links/crawl-request?id=${encodeURIComponent(portalLinkId)}`, {
    credentials: 'include',
  })
  return proxyJsonOrThrow<CrawlRequest>(res, 'load crawl request')
}

// POST browser/crawl — navigate the shared browser session to the
// link's own URL. Must happen before crawlPaginated below, which
// operates on "the currently loaded page."
export async function navigateTo(url: string): Promise<void> {
  const res = await fetch(`${BROWSER_PROXY_BASE}/crawl`, {
    method: 'POST',
    credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ url }),
  })
  await proxyJsonOrThrow<unknown>(res, 'navigate to page')
}

export interface CrawlResultPage {
  url: string
  // Exactly one of results/items is ever present (step 30, mirroring
  // browser's own step 18 contract) — results in flat mode, items (one
  // object per matched container) in grouped mode.
  results?: Record<string, unknown>
  items?: Record<string, unknown>[]
  notFound: string[]
}

export interface CrawlPaginatedResult {
  pages: CrawlResultPage[]
  stoppedReason: string
  pagesVisited: number
  requestedMaxPages: number
  effectiveMaxPages: number
}

// POST browser/crawl-paginated — extract fields[] from the current
// page, follow nextSelector, repeat up to effectiveMaxPages times.
export async function crawlPaginated(request: CrawlRequest): Promise<CrawlPaginatedResult> {
  const res = await fetch(`${BROWSER_PROXY_BASE}/crawl-paginated`, {
    method: 'POST',
    credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(request),
  })
  return proxyJsonOrThrow<CrawlPaginatedResult>(res, 'run paginated crawl')
}

export interface IngestCrawlResultsResult {
  jobsSaved: number
  jobsUpdated: number
  jobsSkipped: number
}

// POST career/portal-links/ingest-crawl-results — maps the crawl's
// own raw pages onto job rows via the fixed label vocabulary (title/
// url required; company/location/description/postedAt optional) and
// saves each one through the same savePortalJob the AI-based path
// already uses.
export async function ingestCrawlResults(portalLinkId: string, pages: CrawlResultPage[]): Promise<IngestCrawlResultsResult> {
  const res = await fetch(`${CAREER_PROXY_BASE}/portal-links/ingest-crawl-results`, {
    method: 'POST',
    credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ portalLinkId, pages }),
  })
  return proxyJsonOrThrow<IngestCrawlResultsResult>(res, 'save crawled jobs')
}
