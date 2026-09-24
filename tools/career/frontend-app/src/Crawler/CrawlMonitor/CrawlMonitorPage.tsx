import { Fragment, useEffect, useRef, useState } from 'react'
import { fetchActiveCrawlRuns, stopCrawlNow, type ActiveCrawlRunSummary } from '../crawlNow'
import { phaseLabel, latestCrawlLogMessage } from '../crawlPhase'
import { StopIcon, LogIcon, AlertIcon } from '../../Shared/Icons/icons'

// CrawlMonitorPage is the centralized "which portal is being crawled
// right now" view — one place to see every currently-running crawl
// (of either kind: "Crawl now" against a listing page, or "Crawl job
// details now" against every already-saved job's own detail page)
// across every portal and link at once, instead of opening each
// portal's own accordion individually. Polls
// GET /portal-links/crawl-runs/active (crawl_monitor.go) — a plain,
// stateless list refresh; unlike CrawlPanel's own per-link watch, there
// is no local optimistic state to reconcile, since every row here
// already IS the server's own live view. See
// plan/ai/tools/career/step-XX-portal-crawl-pacing.md.
const CRAWL_MONITOR_POLL_INTERVAL_MS = 3000

const KIND_LABELS: Record<ActiveCrawlRunSummary['kind'], string> = {
  listing: 'Listing crawl',
  job_detail: 'Job detail crawl',
}

export function CrawlMonitorPage() {
  const [runs, setRuns] = useState<ActiveCrawlRunSummary[]>([])
  const [error, setError] = useState('')
  const [loaded, setLoaded] = useState(false)
  const [expandedRunId, setExpandedRunId] = useState<string | null>(null)
  const [stoppingLinkId, setStoppingLinkId] = useState<string | null>(null)

  function load() {
    fetchActiveCrawlRuns()
      .then((fetched) => {
        setRuns(fetched)
        setError('')
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : 'Failed to load active crawls.')
      })
      .finally(() => {
        setLoaded(true)
      })
  }

  const pollTokenRef = useRef(0)
  useEffect(() => {
    const myToken = pollTokenRef.current + 1
    pollTokenRef.current = myToken
    load()
    const interval = setInterval(() => {
      if (pollTokenRef.current === myToken) load()
    }, CRAWL_MONITOR_POLL_INTERVAL_MS)
    return () => {
      pollTokenRef.current += 1
      clearInterval(interval)
    }
  }, [])

  function handleStop(portalLinkId: string) {
    setStoppingLinkId(portalLinkId)
    stopCrawlNow(portalLinkId)
      .then(load)
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : 'Failed to stop crawl.')
      })
      .finally(() => {
        setStoppingLinkId(null)
      })
  }

  return (
    <div className="max-w-5xl p-6 font-sans text-gray-900">
      <h1 className="mb-1.5 text-xl font-semibold">Crawl Monitor</h1>
      <p className="mb-5 text-sm text-gray-500">Every portal currently being crawled, across every link — refreshes automatically.</p>

      <div className="min-h-[1.2em] text-sm text-red-700">{error}</div>

      {loaded && runs.length === 0 && !error && (
        <div className="rounded-md border border-gray-200 bg-gray-50 p-6 text-center text-sm text-gray-500">
          No crawls are currently running.
        </div>
      )}

      {runs.length > 0 && (
        <div className="overflow-hidden rounded-md border border-gray-200 shadow-sm">
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr>
                  <th className="border-b border-gray-200 bg-gray-50 p-3 text-left font-medium text-gray-500">Portal</th>
                  <th className="border-b border-gray-200 bg-gray-50 p-3 text-left font-medium text-gray-500">Link</th>
                  <th className="border-b border-gray-200 bg-gray-50 p-3 text-left font-medium text-gray-500">Kind</th>
                  <th className="border-b border-gray-200 bg-gray-50 p-3 text-left font-medium text-gray-500">Status</th>
                  <th className="border-b border-gray-200 bg-gray-50 p-3 text-left font-medium text-gray-500">Started</th>
                  <th className="border-b border-gray-200 bg-gray-50 p-3"></th>
                </tr>
              </thead>
              <tbody>
                {runs.map((run) => (
                  <Fragment key={run.crawlRunId}>
                    <tr className="last:[&>td]:border-b-0 hover:bg-gray-50">
                      <td className="border-b border-gray-200 p-3 font-medium">{run.portalName}</td>
                      <td className="border-b border-gray-200 p-3">
                        <a
                          href={run.portalLinkUrl}
                          target="_blank"
                          rel="noreferrer"
                          className="text-gray-900 underline decoration-gray-300 hover:decoration-gray-600"
                        >
                          {run.portalLinkTitle ?? run.portalLinkUrl}
                        </a>
                      </td>
                      <td className="border-b border-gray-200 p-3 text-gray-500">
                        <div className="flex items-center gap-1.5">
                          {KIND_LABELS[run.kind]}
                          {run.triggeredBy === 'auto_discovery' && (
                            <span
                              title="Started automatically by auto-discovery, not a manual click"
                              className="rounded-full bg-gray-200 px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-wide text-gray-600"
                            >
                              Auto
                            </span>
                          )}
                        </div>
                      </td>
                      <td className="border-b border-gray-200 p-3">
                        {run.phase === 'awaiting_human_challenge' ? (
                          <span className="flex items-center gap-1.5 text-amber-800">
                            <AlertIcon />
                            Waiting for a Cloudflare challenge to be solved
                          </span>
                        ) : (
                          <span className="text-gray-500">{phaseLabel(run.phase) ?? latestCrawlLogMessage(run.log) ?? 'Running…'}</span>
                        )}
                      </td>
                      <td className="border-b border-gray-200 p-3 text-gray-500">{new Date(run.startedAt).toLocaleString()}</td>
                      <td className="border-b border-gray-200 p-3">
                        <div className="flex items-center justify-end gap-2">
                          <button
                            type="button"
                            onClick={() => {
                              setExpandedRunId((prev) => (prev === run.crawlRunId ? null : run.crawlRunId))
                            }}
                            title="Show every logged step of this crawl run."
                            className="flex items-center gap-1 rounded-md border border-gray-200 px-2 py-1 text-xs text-gray-700 hover:bg-white"
                          >
                            <LogIcon />
                            {expandedRunId === run.crawlRunId ? 'Hide log' : 'View log'}
                          </button>
                          <button
                            type="button"
                            onClick={() => {
                              handleStop(run.portalLinkId)
                            }}
                            disabled={stoppingLinkId === run.portalLinkId}
                            title="Actually stop this crawl — interrupts it on the server, not just this view."
                            className="flex items-center gap-1 rounded-md border border-gray-200 px-2 py-1 text-xs text-gray-700 hover:bg-white disabled:cursor-not-allowed disabled:opacity-50"
                          >
                            <StopIcon />
                            {stoppingLinkId === run.portalLinkId ? 'Stopping…' : 'Stop'}
                          </button>
                        </div>
                      </td>
                    </tr>
                    {expandedRunId === run.crawlRunId && (
                      <tr>
                        <td colSpan={6} className="border-b border-gray-200 bg-gray-900 p-0">
                          <pre className="max-h-40 overflow-y-auto whitespace-pre-wrap p-3 text-xs text-gray-100">
                            {run.log.length > 0 ? run.log.join('\n') : '(no log entries yet)'}
                          </pre>
                        </td>
                      </tr>
                    )}
                  </Fragment>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}
    </div>
  )
}
