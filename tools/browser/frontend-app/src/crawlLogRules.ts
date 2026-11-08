// Pure rules for the Crawl Logs page's outcome panel. No React, so they can be script-checked.
import type { CrawlLogEntry } from './api'

export type ReasonTone = 'ok' | 'warn' | 'bad' | 'neutral'

// Why a crawl stopped, in words. The backend's codes are snake_case; one this table does not
// know is shown with the underscores turned into spaces.
const REASONS = new Map<string, { label: string; tone: ReasonTone }>([
  ['max_pages_reached', { label: 'Reached the page limit', tone: 'ok' }],
  ['no_next_link', { label: 'No next page', tone: 'ok' }],
  ['time_budget_reached', { label: 'Ran out of time', tone: 'warn' }],
  ['url_unchanged', { label: 'Page did not change', tone: 'warn' }],
  ['click_failed', { label: 'Next click failed', tone: 'bad' }],
  ['cloudflare_blocked', { label: 'Blocked by a check', tone: 'bad' }],
])

export function reasonLabel(code: string): string {
  const known = REASONS.get(code)
  if (known) return known.label
  const words = code.replace(/_/g, ' ').trim()
  return words === '' ? 'Unknown' : words.charAt(0).toUpperCase() + words.slice(1)
}

export function reasonTone(code: string): ReasonTone {
  return REASONS.get(code)?.tone ?? 'neutral'
}

export interface ReasonCount {
  code: string
  label: string
  tone: ReasonTone
  count: number
  // Share of all crawls, in percent.
  percent: number
}

export interface CrawlLogSummary {
  total: number
  // Pages visited over all crawls.
  pages: number
  // Crawls in which at least one requested field matched nothing.
  withUnmatched: number
  reasons: ReasonCount[]
}

export function summarizeCrawlLogs(entries: CrawlLogEntry[]): CrawlLogSummary {
  const counts = new Map<string, number>()
  let pages = 0
  let withUnmatched = 0
  for (const e of entries) {
    counts.set(e.stoppedReason, (counts.get(e.stoppedReason) ?? 0) + 1)
    pages += e.pagesVisited
    if (e.notFoundCount > 0) withUnmatched++
  }
  const total = entries.length
  const reasons = [...counts.entries()]
    .map(([code, count]) => ({
      code,
      label: reasonLabel(code),
      tone: reasonTone(code),
      count,
      percent: total === 0 ? 0 : (count / total) * 100,
    }))
    .sort((a, b) => b.count - a.count || a.label.localeCompare(b.label))
  return { total, pages, withUnmatched, reasons }
}

// The list to show: all crawls, or only those with a field that matched nothing.
export function visibleLogs<T extends { notFoundCount: number }>(entries: T[], onlyUnmatched: boolean): T[] {
  return onlyUnmatched ? entries.filter((e) => e.notFoundCount > 0) : entries
}

// What bridge.openFileReader takes (taken from the bridge's own type copy in cinqo-tool-bridge.d.ts).
type ReaderOptions = Parameters<NonNullable<Window['__cinqoToolBridge']['openFileReader']>>[0]

// The Browser's file source in manifest.json "file_sources" (plan 201).
export const CRAWL_LOG_SOURCE = 'crawl-logs'

// What the "Read in the log reader" button asks Cinqo to open for a crawl: its raw file, as a page of the reader (the return link is the
// page the person is on, so it needs no address here). null when this crawl has no file the reader can open, so no button is drawn.
export function readerOptionsFor(entry: CrawlLogEntry): ReaderOptions | null {
  if (!entry.logRef) return null
  return {
    tool: 'browser',
    source: CRAWL_LOG_SOURCE,
    refs: [entry.logRef],
    title: `Crawl log ${entry.id.slice(0, 8)}`,
    // the file is JSON (one compact line): the reader beautifies it, and the person can switch to text
    format: 'json',
    backLabel: 'Crawl Logs',
  }
}

// Whether the Cinqo hosting this page can open the reader page (a host older than plan 201 has no such call, so the button is hidden).
export function hostCanOpenReader(bridge: { openFileReader?: unknown } | undefined): boolean {
  return typeof bridge?.openFileReader === 'function'
}

// The reader page opens at most this many files at once (the same limit Cinqo applies to the address).
export const MAX_READER_FILES = 20

// What "Read selected" asks Cinqo to open: the raw files of the selected crawls, in the order the list shows them (newest first), each
// only if it has a file the reader can open, at most MAX_READER_FILES. The first one opens first. null when nothing readable is selected.
export function readerOptionsForMany(logs: CrawlLogEntry[], selectedIds: ReadonlySet<string>): ReaderOptions | null {
  const refs = logs
    .filter((e) => selectedIds.has(e.id) && e.logRef)
    .map((e) => e.logRef ?? '')
    .slice(0, MAX_READER_FILES)
  if (refs.length === 0) return null
  if (refs.length === 1)
    return { tool: 'browser', source: CRAWL_LOG_SOURCE, refs, title: 'Crawl log', format: 'json', backLabel: 'Crawl Logs' }
  return { tool: 'browser', source: CRAWL_LOG_SOURCE, refs, title: `${refs.length} crawl logs`, format: 'json', backLabel: 'Crawl Logs' }
}

// How many of the selected crawls are still in the list (a deleted or rotated-out one drops out of the selection by itself).
export function selectedCount(logs: CrawlLogEntry[], selectedIds: ReadonlySet<string>): number {
  return logs.filter((e) => selectedIds.has(e.id) && e.logRef).length
}

// Whether one more row may be ticked: not past the limit, and the row has a file at all.
export function canTick(entry: CrawlLogEntry, count: number, selectedIds: ReadonlySet<string>): boolean {
  if (!entry.logRef) return false
  return selectedIds.has(entry.id) || count < MAX_READER_FILES
}
