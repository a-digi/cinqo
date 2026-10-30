import { useEffect, useState } from 'react'
import { clearAllCrawlLogs, deleteCrawlLog, fetchCrawlLogDetail, fetchCrawlLogs, type CrawlLogDetail, type CrawlLogEntry } from './api'
import { reasonLabel, reasonTone, summarizeCrawlLogs, visibleLogs, type ReasonTone } from './crawlLogRules'
import { ListIcon } from './icons'
import { OutcomePanel } from './OutcomePanel'
import { PageHeaderCard } from './Shared/PageHeaderCard'
import { RawLog } from './RawLog'
import { plural } from './Shared/plural'
import { PAGE_ACCENT } from './Shared/theme'

const PILL: Record<ReasonTone, string> = {
  ok: 'bg-green-100 text-green-800',
  warn: 'bg-amber-100 text-amber-800',
  bad: 'bg-red-100 text-red-800',
  neutral: 'bg-gray-100 text-gray-700',
}

// Read-only diagnostic viewer for step 17 — every past /crawl-paginated
// call (both career's own deterministic "Crawl now" and the AI's own
// crawl_paginated tool calls go through the same handler this logs),
// showing exactly what was asked for and what the page actually gave
// back, including which field labels matched nothing. See
// plan/ai/tools/browser/step-17-crawl-diagnostic-logging.md.
//
// Step 39 — the list call only ever returns each entry's own
// lightweight summary now (firstPageUrl/notFoundCount computed
// server-side, replacing what used to be computed here client-side
// against the full payload); expanding a row fetches and caches that
// one entry's full detail (including any captured HTML) on demand,
// via fetchCrawlLogDetail, instead of the list call already carrying
// every entry's full body whether or not it was ever expanded. See
// plan/ai/tools/browser/step-39-crawl-log-file-storage.md. An outcome panel on the
// right (why crawls stopped, pages visited, crawls with fields not found) is the
// overview.
export function CrawlLogsPage() {
  const [logs, setLogs] = useState<CrawlLogEntry[]>([])
  const [error, setError] = useState('')
  const [expandedId, setExpandedId] = useState<string | null>(null)
  const [details, setDetails] = useState<Record<string, CrawlLogDetail | undefined>>({})
  const [detailErrors, setDetailErrors] = useState<Record<string, string | undefined>>({})
  const [loadingIds, setLoadingIds] = useState<Set<string>>(new Set())
  // Show only the crawls with a field that matched nothing.
  const [onlyUnmatched, setOnlyUnmatched] = useState(false)

  function load() {
    fetchCrawlLogs()
      .then(setLogs)
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
  }

  useEffect(load, [])

  function toggleExpand(id: string) {
    const next = expandedId === id ? null : id
    setExpandedId(next)
    if (!next || details[next] || loadingIds.has(next)) return

    setLoadingIds((prev) => new Set(prev).add(next))
    setDetailErrors((prev) => ({ ...prev, [next]: undefined }))
    fetchCrawlLogDetail(next)
      .then((detail) => {
        setDetails((prev) => ({ ...prev, [next]: detail }))
      })
      .catch((err: unknown) => {
        setDetailErrors((prev) => ({ ...prev, [next]: err instanceof Error ? err.message : String(err) }))
      })
      .finally(() => {
        setLoadingIds((prev) => {
          const nextSet = new Set(prev)
          nextSet.delete(next)
          return nextSet
        })
      })
  }

  function handleDelete(id: string) {
    setError('')
    deleteCrawlLog(id)
      .then(load)
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
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
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
  }

  const summary = summarizeCrawlLogs(logs)
  const shown = visibleLogs(logs, onlyUnmatched)

  return (
    <div className="browser-v-right browser-a-float browser-t-type max-w-6xl p-6 font-sans text-gray-900">
      <PageHeaderCard
        icon={<ListIcon />}
        title="Crawl Logs"
        description={`The most recent paginated crawls — what fields/selectors were used, which URLs were actually visited, what was extracted, and which field labels matched nothing on the page. Kept for the last ${logs.length > 0 ? 'up to 100' : '100'} crawls, newest first. Nothing is logged here unless Debug is turned on (see the Debug page).`}
        pill={{ text: plural(logs.length, 'crawl', 'crawls') }}
      />

      <div className="flex flex-col gap-5 lg:flex-row lg:items-start">
        <div className="min-w-0 flex-1">
          <div className="mb-3 flex items-center justify-between gap-3">
            <p className="text-xs text-gray-500">
              {onlyUnmatched
                ? `Showing ${shown.length} of ${logs.length}: only crawls with fields not found.`
                : 'Click a crawl to see what it did.'}
            </p>
            {logs.length > 0 && (
              <button
                type="button"
                onClick={handleClearAll}
                className="shrink-0 rounded-full border border-red-200 px-3 py-1 text-xs text-red-700 hover:bg-red-50"
              >
                Clear all
              </button>
            )}
          </div>

          <div className="min-h-[1.2em] text-sm text-red-700">{error}</div>

          {logs.length === 0 && !error && (
            <div
              className="browser-wiggle rounded-3xl border p-8 text-center text-sm text-gray-500"
              style={{ borderColor: PAGE_ACCENT.soft, background: PAGE_ACCENT.soft }}
            >
              No crawls logged yet.
            </div>
          )}

          <div className="browser-stagger space-y-3">
            {shown.map((entry) => {
              const expanded = expandedId === entry.id
              const detail = details[entry.id]
              return (
                <div
                  key={entry.id}
                  className="rounded-3xl border p-4 shadow-sm"
                  style={{ borderColor: PAGE_ACCENT.soft, background: `linear-gradient(160deg, #ffffff 45%, ${PAGE_ACCENT.soft} 100%)` }}
                >
                  <div className="flex w-full items-center justify-between gap-3">
                    <button
                      type="button"
                      onClick={() => {
                        toggleExpand(entry.id)
                      }}
                      className="flex min-w-0 flex-1 items-center justify-between gap-3 text-left"
                    >
                      <div className="min-w-0">
                        <div className="truncate text-sm font-semibold text-gray-900">{entry.firstPageUrl ?? '(no page reached)'}</div>
                        <div className="mt-1 flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-gray-500">
                          <span
                            className={`rounded-full px-2 py-0.5 font-medium ${PILL[reasonTone(entry.stoppedReason)]}`}
                            title={entry.stoppedReason}
                          >
                            {reasonLabel(entry.stoppedReason)}
                          </span>
                          <span>
                            {new Date(entry.createdAt).toLocaleString()} · {entry.pagesVisited} page(s)
                          </span>
                          {entry.notFoundCount > 0 && <span className="text-red-700">{entry.notFoundCount} field(s) not found</span>}
                        </div>
                      </div>
                      <span className="shrink-0 text-xs text-blue-700 underline">{expanded ? 'hide' : 'view details'}</span>
                    </button>
                    <button
                      type="button"
                      onClick={() => {
                        handleDelete(entry.id)
                      }}
                      className="shrink-0 rounded-full border border-red-200 px-3 py-1 text-xs text-red-700 hover:bg-red-50"
                    >
                      delete
                    </button>
                  </div>

                  {expanded && (
                    <div className="mt-3 space-y-3 border-t border-gray-100 pt-3">
                      {loadingIds.has(entry.id) && <p className="text-xs text-gray-400">Loading…</p>}
                      {detailErrors[entry.id] && <p className="text-xs text-red-700">{detailErrors[entry.id]}</p>}
                      {detail && (
                        <>
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
                                {detail.fields.map((f, i) => (
                                  <tr key={i}>
                                    <td className="pr-2 font-mono">{f.label}</td>
                                    <td className="pr-2 font-mono">{f.selector}</td>
                                    <td className="pr-2 font-mono">{f.attribute ?? '(text)'}</td>
                                    <td>{f.multiple ? 'yes' : 'no'}</td>
                                  </tr>
                                ))}
                              </tbody>
                            </table>
                            <div className="mt-1 text-xs text-gray-500">
                              Pagination: <span className="font-mono">{detail.nextSelector}</span>, requested {detail.requestedMaxPages}{' '}
                              page(s), capped at {detail.effectiveMaxPages}
                            </div>
                            {detail.container && (
                              <div className="mt-1 text-xs text-gray-500">
                                Container (grouped extraction): <span className="font-mono">{detail.container}</span>
                              </div>
                            )}
                          </div>

                          {detail.pages.map((page, i) => (
                            <div key={i} className="rounded bg-gray-50 p-2">
                              <div className="mb-1 truncate text-xs font-medium text-gray-900">
                                Page {i + 1}: {page.url}
                                {page.items && <span className="ml-1.5 font-normal text-gray-500">({page.items.length} item(s))</span>}
                              </div>
                              {page.cloudflareDetected && (
                                <p className="mb-1 text-xs font-medium text-amber-700">
                                  ⚠️ Cloudflare challenge detected on this page (reason: {page.cloudflareReason}) — results below may
                                  reflect the challenge interstitial, not real content.
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
                        </>
                      )}
                      <RawLog id={entry.id} />
                    </div>
                  )}
                </div>
              )
            })}
          </div>
        </div>

        <aside className="shrink-0 lg:sticky lg:top-6 lg:w-64">
          <OutcomePanel summary={summary} onlyUnmatched={onlyUnmatched} onToggleUnmatched={setOnlyUnmatched} />
        </aside>
      </div>
    </div>
  )
}
