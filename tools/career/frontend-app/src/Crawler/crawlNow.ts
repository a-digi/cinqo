// crawlNow.ts — the "Crawl now" mechanism's own plain proxy calls
// (steps 37/38): one POST to start a detached crawl on Career's own
// backend, and a GET to read its live status/log/result. Replaces
// browserApi.ts's own fetchCrawlRequest/navigateTo/crawlPaginated/
// ingestCrawlResults entirely — that sequencing now runs server-side
// (tools/career/backend/crawl_now.go), not in this file or any other
// browser tab. credentials:'include' is the only thing this file needs
// to do with authentication — Career's own backend reads the same
// session cookie directly off the inbound POST /portal-links/crawl-now
// request to authenticate its own later, detached calls to the browser
// tool; nothing about that is visible here. See
// plan/ai/tools/career/step-37-detached-crawl-now-orchestration.md and
// plan/ai/tools/career/step-38-crawl-now-polling-frontend.md.

const CRAWL_NOW_BASE = '/api/v1/tools/career/proxy/portal-links/crawl-now'

export interface StartedCrawlRun {
  crawlRunId: string
  status: 'running'
  startedAt: string
}

export interface CrawlRun {
  crawlRunId: string
  status: 'running' | 'completed' | 'failed' | 'cancelled'
  startedAt: string
  finishedAt: string | null
  log: string[]
  resultSummary: string | null
  errorMessage: string | null
  // phase (step 39) is the single most recent fine-grained step this
  // run has reached — a plain string, not a TS union: it only ever
  // needs a lookup-table label (see PortalsPage.tsx's own
  // PHASE_LABELS) and a special case for 'awaiting_human_challenge',
  // and a union would need updating here every time the backend's own
  // fixed vocabulary changes, for no real type-safety benefit (the
  // value always comes from the network). null until the first phase
  // transition lands, never cleared afterward. See
  // plan/ai/tools/career/step-40-phase-display-frontend.md.
  phase: string | null
}

// startCrawlNow starts a detached crawl for this link — 409 (a crawl
// already running for it) and any other failure are both surfaced as
// a plain Error; the caller decides where to display it (PortalsPage's
// own existing crawlResults notice, not this file's concern).
export async function startCrawlNow(portalLinkId: string): Promise<StartedCrawlRun> {
  const res = await fetch(CRAWL_NOW_BASE, {
    method: 'POST',
    credentials: 'include',
    body: JSON.stringify({ portalLinkId }),
  })
  if (res.status === 409) throw new Error('A crawl is already running for this link.')
  if (!res.ok) {
    const text = await res.text().catch(() => '')
    throw new Error(text || `failed to start crawl (${res.status})`)
  }
  return res.json() as Promise<StartedCrawlRun>
}

// fetchActiveCrawlRun returns the most recent crawl run for this link
// — running or terminal — or null if none has ever existed. Mirrors
// the AI conversation feature's own GET .../turns/active contract:
// still returns the row once more on a running -> terminal
// transition, so a poller reliably observes completion instead of
// racing a 404.
export async function fetchActiveCrawlRun(portalLinkId: string): Promise<CrawlRun | null> {
  const res = await fetch(`${CRAWL_NOW_BASE}/active?portalLinkId=${encodeURIComponent(portalLinkId)}`, {
    credentials: 'include',
  })
  if (res.status === 404) return null
  if (!res.ok) throw new Error(`failed to load crawl status (${res.status})`)
  return res.json() as Promise<CrawlRun>
}
