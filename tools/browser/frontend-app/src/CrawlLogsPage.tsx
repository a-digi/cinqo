import { useEffect, useState } from 'react'
import { clearAllCrawlLogs, deleteCrawlLog, fetchCrawlLogs, type CrawlLogEntry } from './api'

// Read-only diagnostic viewer for step 17 — every past /crawl-paginated
// call (both career's own deterministic "Crawl now" and the AI's own
// crawl_paginated tool calls go through the same handler this logs),
// showing exactly what was asked for and what the page actually gave
// back, including which field labels matched nothing. See
// plan/ai/tools/browser/step-17-crawl-diagnostic-logging.md.
export function CrawlLogsPage() {
  const [logs, setLogs] = useState<CrawlLogEntry[]>([])
  const [error, setError] = useState('')
  const [expandedId, setExpandedId] = useState<string | null>(null)

  function load() {
    fetchCrawlLogs()
      .then(setLogs)
      .catch((err: Error) => setError(err.message))
  }

  useEffect(load, [])

  function handleDelete(id: string) {
    setError('')
    deleteCrawlLog(id)
      .then(load)
      .catch((err: Error) => setError(err.message))
  }

  function handleClearAll() {
    // Unlike a single entry's own delete button (matching this app's
    // existing no-confirm precedent for e.g. removing a credential),
    // this is bulk and irreversible — a deliberate divergence from that
    // precedent, not an oversight.
    if (!window.confirm('Delete all crawl logs? This cannot be undone.')) return
    setError('')
    clearAllCrawlLogs()
      .then(load)
      .catch((err: Error) => setError(err.message))
  }

  return (
    <div className="max-w-3xl p-6 font-sans text-gray-900">
      <div className="mb-1.5 flex items-center justify-between gap-3">
        <h1 className="text-xl font-semibold">Crawl Logs</h1>
        {logs.length > 0 && (
          <button
            type="button"
            onClick={handleClearAll}
            className="shrink-0 text-xs text-red-700 underline"
          >
            Clear all
          </button>
        )}
      </div>
      <p className="mb-5 text-sm text-gray-500">
        The most recent paginated crawls — what fields/selectors were used, which URLs were actually
        visited, what was extracted, and which field labels matched nothing on the page. Kept for the
        last {logs.length > 0 ? 'up to 100' : '100'} crawls, newest first. Nothing is logged here
        unless Debug is turned on (see the Debug page).
      </p>

      <div className="min-h-[1.2em] text-sm text-red-700">{error}</div>

      {logs.length === 0 && !error && <p className="text-sm text-gray-400">No crawls logged yet.</p>}

      <div className="space-y-2">
        {logs.map((entry) => {
          const expanded = expandedId === entry.id
          const totalNotFound = entry.pages.reduce((n, p) => n + p.notFound.length, 0)
          return (
            <div key={entry.id} className="rounded-md border border-gray-200 p-3">
              <div className="flex w-full items-center justify-between gap-3">
                <button
                  type="button"
                  onClick={() => setExpandedId(expanded ? null : entry.id)}
                  className="flex min-w-0 flex-1 items-center justify-between gap-3 text-left"
                >
                  <div className="min-w-0">
                    <div className="truncate text-xs font-medium text-gray-900">
                      {entry.pages[0]?.url || '(no page reached)'}
                    </div>
                    <div className="text-xs text-gray-500">
                      {new Date(entry.createdAt).toLocaleString()} · {entry.pagesVisited} page(s) ·{' '}
                      {entry.stoppedReason}
                      {totalNotFound > 0 && <span className="text-red-700"> · {totalNotFound} field(s) not found</span>}
                    </div>
                  </div>
                  <span className="shrink-0 text-xs text-gray-500 underline">{expanded ? 'hide' : 'view details'}</span>
                </button>
                <button
                  type="button"
                  onClick={() => handleDelete(entry.id)}
                  className="shrink-0 text-xs text-red-700 underline"
                >
                  delete
                </button>
              </div>

              {expanded && (
                <div className="mt-3 space-y-3 border-t border-gray-100 pt-3">
                  <div>
                    <div className="mb-1 text-xs font-medium text-gray-500">Instructions used</div>
                    <table className="w-full text-xs">
                      <thead>
                        <tr className="text-left text-gray-500">
                          <th className="pb-1 pr-2">Label</th>
                          <th className="pb-1 pr-2">Selector</th>
                          <th className="pb-1 pr-2">Attribute</th>
                          <th className="pb-1">Multiple</th>
                        </tr>
                      </thead>
                      <tbody>
                        {entry.fields.map((f, i) => (
                          <tr key={i}>
                            <td className="pr-2 font-mono">{f.label}</td>
                            <td className="pr-2 font-mono">{f.selector}</td>
                            <td className="pr-2 font-mono">{f.attribute || '(text)'}</td>
                            <td>{f.multiple ? 'yes' : 'no'}</td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                    <div className="mt-1 text-xs text-gray-500">
                      Pagination: <span className="font-mono">{entry.nextSelector}</span>, requested{' '}
                      {entry.requestedMaxPages} page(s), capped at {entry.effectiveMaxPages}
                    </div>
                    {entry.container && (
                      <div className="mt-1 text-xs text-gray-500">
                        Container (grouped extraction): <span className="font-mono">{entry.container}</span>
                      </div>
                    )}
                  </div>

                  {entry.pages.map((page, i) => (
                    <div key={i} className="rounded bg-gray-50 p-2">
                      <div className="mb-1 truncate text-xs font-medium text-gray-900">
                        Page {i + 1}: {page.url}
                        {page.items && <span className="ml-1.5 font-normal text-gray-500">({page.items.length} item(s))</span>}
                      </div>
                      {page.cloudflareDetected && (
                        <p className="mb-1 text-xs font-medium text-amber-700">
                          ⚠️ Cloudflare challenge detected on this page (reason: {page.cloudflareReason}) — results below may reflect the challenge interstitial, not real content.
                        </p>
                      )}
                      <pre className="overflow-x-auto whitespace-pre-wrap break-words text-xs text-gray-700">
                        {JSON.stringify(page.items ?? page.results, null, 2)}
                      </pre>
                      {page.notFound.length > 0 && (
                        <p className="mt-1 text-xs text-red-700">Not found on this page: {page.notFound.join(', ')}</p>
                      )}
                      {page.html && (
                        <details className="mt-1">
                          <summary className="cursor-pointer text-xs text-gray-500 underline">
                            Raw HTML ("Log HTML" was on for this crawl)
                          </summary>
                          <pre className="mt-1 max-h-64 overflow-auto whitespace-pre-wrap break-words rounded bg-white p-2 text-xs text-gray-700">
                            {page.html}
                          </pre>
                        </details>
                      )}
                    </div>
                  ))}
                </div>
              )}
            </div>
          )
        })}
      </div>
    </div>
  )
}
