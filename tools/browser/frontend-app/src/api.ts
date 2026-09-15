// api.ts — the same GET/POST/DELETE /login-credentials calls steps
// 7/10 already built, reused verbatim. The backend never inspects
// Content-Type on POST and JSON is valid YAML (verified directly
// while designing step 10), so a plain JSON.stringify body is safe —
// no hand-templated YAML, no injection risk from a domain/username/
// password containing YAML-special characters.
const PROXY_BASE = '/api/v1/tools/browser/proxy/login-credentials'

export interface CredentialSummary {
  domain: string
  username: string
}

interface CredentialsListResponse {
  credentials: CredentialSummary[]
}

export async function fetchCredentials(): Promise<CredentialSummary[]> {
  const res = await fetch(PROXY_BASE, { credentials: 'include' })
  if (!res.ok) throw new Error(`failed to load credentials (${res.status})`)
  const data: CredentialsListResponse = await res.json()
  return data.credentials ?? []
}

export async function saveCredential(domain: string, username: string, password: string): Promise<CredentialSummary[]> {
  const res = await fetch(PROXY_BASE, {
    method: 'POST',
    credentials: 'include',
    body: JSON.stringify({ login_credentials: [{ domain, username, password }] }),
  })
  if (!res.ok) throw new Error(`failed to save credential (${res.status})`)
  const data: CredentialsListResponse = await res.json()
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
}

export interface CrawlLogEntry {
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
  const data: { logs: CrawlLogEntry[] } = await res.json()
  return data.logs ?? []
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
