// api.ts — the same GET/POST/DELETE /login-credentials calls steps
// 7/10 already built, reused verbatim. The backend never inspects
// Content-Type on POST and JSON is valid YAML (verified directly
// while designing step 10), so a plain JSON.stringify body is safe —
// no hand-templated YAML, no injection risk from a domain/username/
// password containing YAML-special characters.
// res.json() is typed Promise<any> — this is the one place that's cast to
// a concrete type, so every call site below gets a real type without its
// own scattered unsafe assignment.
async function readJson<T>(res: Response): Promise<T> {
  return (await res.json()) as T
}

const PROXY_BASE = '/api/v1/tools/browser/proxy/login-credentials'

export interface CredentialSummary {
  domain: string
  username: string
}

interface CredentialsListResponse {
  // Optional, not required — matches the `data.credentials ?? []` at
  // every call site: a Go `omitempty` slice field is genuinely absent
  // from the JSON when there are zero credentials, not sent as `[]`.
  credentials?: CredentialSummary[]
}

export async function fetchCredentials(): Promise<CredentialSummary[]> {
  const res = await fetch(PROXY_BASE, { credentials: 'include' })
  if (!res.ok) throw new Error(`failed to load credentials (${res.status})`)
  const data = await readJson<CredentialsListResponse>(res)
  return data.credentials ?? []
}

export async function saveCredential(domain: string, username: string, password: string): Promise<CredentialSummary[]> {
  const res = await fetch(PROXY_BASE, {
    method: 'POST',
    credentials: 'include',
    body: JSON.stringify({ login_credentials: [{ domain, username, password }] }),
  })
  if (!res.ok) throw new Error(`failed to save credential (${res.status})`)
  const data = await readJson<CredentialsListResponse>(res)
  return data.credentials ?? []
}

// --- crawl logs (step 17) ---

const CRAWL_LOGS_BASE = '/api/v1/tools/browser/proxy/crawl-logs'

export interface CrawlLogField {
  label: string
  selector: string
  attribute?: string
  multiple?: boolean
}

export interface CrawlLogPage {
  url: string
  // Exactly one of results/items is ever present (step 18) — results
  // in flat mode, items (one object per matched container) in grouped
  // mode.
  results?: Record<string, unknown>
  items?: Record<string, unknown>[]
  notFound: string[]
  // Set (step 21) when a Cloudflare challenge interstitial was still
  // showing on this page — results/items above may reflect the
  // challenge page, not real content.
  cloudflareDetected?: boolean
  cloudflareReason?: string
  // Set (step 22) only when Debug + "Log HTML" were both on for this
  // crawl — this page's own raw rendered HTML at the time it was read.
  html?: string
}

// CrawlLogEntry (step 39) is the lightweight list-row shape —
// everything listCrawlLogs' own collapsed row needs (backend/crawl_log.go's
// own crawlLogSummary), computed server-side at write time so fetching
// the list never has to open a single log file. The full body (fields/
// results/notFound/Cloudflare flags/HTML) is fetched separately, only
// for whichever one row is actually expanded — see CrawlLogDetail and
// fetchCrawlLogDetail below. See
// plan/ai/tools/browser/step-39-crawl-log-file-storage.md.
export interface CrawlLogEntry {
  id: string
  createdAt: string
  stoppedReason: string
  pagesVisited: number
  firstPageUrl?: string
  notFoundCount: number
}

// CrawlLogDetail is one entry's full body — fetched on demand
// (fetchCrawlLogDetail), never as part of the list.
export interface CrawlLogDetail {
  id: string
  createdAt: string
  // Empty when this crawl used flat (non-grouped) extraction — see
  // CrawlLogPage's own results/items split above.
  container?: string
  fields: CrawlLogField[]
  nextSelector: string
  requestedMaxPages: number
  effectiveMaxPages: number
  pages: CrawlLogPage[]
  stoppedReason: string
  pagesVisited: number
}

export async function fetchCrawlLogs(): Promise<CrawlLogEntry[]> {
  const res = await fetch(CRAWL_LOGS_BASE, { credentials: 'include' })
  if (!res.ok) throw new Error(`failed to load crawl logs (${res.status})`)
  const data = await readJson<{ logs?: CrawlLogEntry[] }>(res)
  return data.logs ?? []
}

// fetchCrawlLogDetail (step 39) — GET ?id=<id> streams that one
// entry's full file straight from disk on the backend
// (http.ServeContent, crawlLogsHandler), so a huge captured-HTML entry
// is only ever fetched when a caller actually expands that specific
// row, never as part of the list.
export async function fetchCrawlLogDetail(id: string): Promise<CrawlLogDetail> {
  const res = await fetch(`${CRAWL_LOGS_BASE}?id=${encodeURIComponent(id)}`, { credentials: 'include' })
  if (!res.ok) throw new Error(`failed to load crawl log (${res.status})`)
  return readJson<CrawlLogDetail>(res)
}

// deleteCrawlLog/clearAllCrawlLogs (step 22) — clearAllCrawlLogs
// requires the explicit all=true query parameter server-side (see
// crawlLogsHandler, crawl_log.go); there's no bare "delete everything"
// default to guard against on this side either.
export async function deleteCrawlLog(id: string): Promise<void> {
  const res = await fetch(`${CRAWL_LOGS_BASE}?id=${encodeURIComponent(id)}`, {
    method: 'DELETE',
    credentials: 'include',
  })
  if (!res.ok && res.status !== 204) throw new Error(`failed to delete crawl log (${res.status})`)
}

export async function clearAllCrawlLogs(): Promise<void> {
  const res = await fetch(`${CRAWL_LOGS_BASE}?all=true`, {
    method: 'DELETE',
    credentials: 'include',
  })
  if (!res.ok && res.status !== 204) throw new Error(`failed to clear crawl logs (${res.status})`)
}

// --- Debug settings (step 22) ---

const SETTINGS_BASE = '/api/v1/tools/browser/proxy/browser-settings'

export interface BrowserSettings {
  debugEnabled: boolean
  debugLogHtml: boolean
}

export async function fetchBrowserSettings(): Promise<BrowserSettings> {
  const res = await fetch(SETTINGS_BASE, { credentials: 'include' })
  if (!res.ok) throw new Error(`failed to load settings (${res.status})`)
  return readJson<BrowserSettings>(res)
}

export async function saveBrowserSettings(settings: BrowserSettings): Promise<BrowserSettings> {
  const res = await fetch(SETTINGS_BASE, {
    method: 'PUT',
    credentials: 'include',
    body: JSON.stringify(settings),
  })
  if (!res.ok) throw new Error(`failed to save settings (${res.status})`)
  return readJson<BrowserSettings>(res)
}

// --- Cloudflare domain cache (step 30) ---
//
// Read-only view of what step 29's own cache (tools/browser/backend/
// cloudflare_domains.go) has recorded — every domain that's ever given
// the headless session an unresolved Cloudflare challenge, and so now
// skips straight to the headed fallback on every crawl. See
// plan/ai/tools/browser/step-30-cloudflare-domain-list-visibility.md.

const CLOUDFLARE_DOMAINS_BASE = '/api/v1/tools/browser/proxy/cloudflare-domains'

export interface CloudflareDomain {
  domain: string
  reason: string
  firstDetectedAt: string
}

export async function fetchCloudflareDomains(): Promise<CloudflareDomain[]> {
  const res = await fetch(CLOUDFLARE_DOMAINS_BASE, { credentials: 'include' })
  if (!res.ok) throw new Error(`failed to load cloudflare domains (${res.status})`)
  const data = await readJson<{ domains?: CloudflareDomain[] }>(res)
  return data.domains ?? []
}

export async function removeCredential(domain: string): Promise<void> {
  const res = await fetch(`${PROXY_BASE}?domain=${encodeURIComponent(domain)}`, {
    method: 'DELETE',
    credentials: 'include',
  })
  if (!res.ok && res.status !== 204) throw new Error(`failed to remove credential (${res.status})`)
}

// normalizeDomain lets a user paste a full login URL (e.g.
// "https://example.com/login") into the domain field instead of
// requiring a bare hostname — a frontend-only convenience, doesn't
// touch the backend/schema at all. Falls back to the raw input
// whenever it isn't a parseable absolute URL.
export function normalizeDomain(raw: string): string {
  try {
    const u = new URL(raw)
    if (u.hostname) return u.hostname
  } catch {
    // Not a parseable absolute URL — treat raw as a bare domain.
  }
  return raw
}
