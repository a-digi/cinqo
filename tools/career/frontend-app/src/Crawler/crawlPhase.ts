// crawlPhase.ts — shared display helpers for a CrawlRun's own phase/log
// (crawlNow.ts), extracted out of CrawlPanel.tsx (step 40's own
// PHASE_LABELS/phaseLabel, plus latestCrawlLogMessage) so the Crawl
// Monitor page (CrawlMonitor/CrawlMonitorPage.tsx) can show the exact
// same human-readable phase text for a run without duplicating this
// lookup table. See plan/ai/tools/career/step-XX-portal-crawl-pacing.md.

// PHASE_LABELS (step 40) — a human-readable sentence per fine-grained
// crawl_runs.phase value (step 39). A plain lookup, not a switch,
// since CrawlRun.phase (crawlNow.ts) is deliberately a plain string,
// not a TS union — see that field's own doc comment.
const PHASE_LABELS: Record<string, string> = {
  building_request: 'Preparing crawl instructions',
  navigating: 'Opening the page',
  checking_cloudflare: 'Checking for a Cloudflare challenge',
  awaiting_human_challenge: 'Waiting for you to solve a Cloudflare challenge',
  extracting: 'Extracting job listings',
  ingesting_jobs: 'Saving jobs',
}

// phaseLabel falls back to the raw phase string for a value not yet in
// PHASE_LABELS (e.g. a phase the backend adds later than this table) —
// still shows *something* meaningful rather than nothing.
export function phaseLabel(phase: string | null): string | null {
  if (!phase) return null
  return PHASE_LABELS[phase] ?? phase
}

// latestCrawlLogMessage strips the leading "RFC3339<TAB>" the
// backend's own log entries carry (crawl_runs.go's appendCrawlRunLog)
// and returns just the human-readable message part of the most recent
// one.
export function latestCrawlLogMessage(log: string[]): string | null {
  if (log.length === 0) return null
  const last = log[log.length - 1]
  const tabIndex = last.indexOf('\t')
  return tabIndex >= 0 ? last.slice(tabIndex + 1) : last
}
