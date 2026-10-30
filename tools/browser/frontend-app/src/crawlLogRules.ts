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
