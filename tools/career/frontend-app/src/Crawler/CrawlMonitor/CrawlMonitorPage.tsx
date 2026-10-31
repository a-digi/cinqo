import { Fragment, useEffect, useRef, useState } from 'react'
import { fetchActiveCrawlRuns, stopCrawlNow, type ActiveCrawlRunSummary } from '../crawlNow'
import { phaseLabel, latestCrawlLogMessage } from '../crawlPhase'
import { StopIcon, LogIcon, AlertIcon, PulseIcon } from '../../Shared/Icons/icons'
import { PageHeaderCard } from '../../Shared/PageHeader/PageHeaderCard'
import { SummaryRail } from '../../Shared/SummaryRail/SummaryRail'
import { ACCENTS, accentFor, initialOf } from '../../Components/JobsPage/jobsTheme'
import { PageTips } from '../../Shared/PageTips/PageTips'
import { PORTALS_PATH } from '../../paths'

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
  listing: 'Listing',
  job_detail: 'Job detail',
}

// "20:53" for a crawl that started today, "29.10. 20:53" for an older one; the full date
// and time are in the tooltip.
function shortStarted(iso: string): string {
  const d = new Date(iso)
  const time = d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
  return d.toDateString() === new Date().toDateString() ? time : `${d.toLocaleDateString([], { day: '2-digit', month: '2-digit' })} ${time}`
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

  const listing = runs.filter((r) => r.kind === 'listing').length
  const detail = runs.length - listing
  const waiting = runs.filter((r) => r.phase === 'awaiting_human_challenge').length
  const automatic = runs.filter((r) => r.triggeredBy === 'auto_discovery').length

  return (
    <div className="career-v-blur career-a-glow career-t-type max-w-7xl p-6 font-sans text-gray-900">
      <PageHeaderCard
        icon={<PulseIcon />}
        title="Crawl Monitor"
        description="Every portal currently being crawled, across every link. Refreshes automatically."
        // The pill links on to the Portals page, where crawls are started and managed.
        pill={{
          text: runs.length === 0 ? 'Idle' : `${runs.length} running`,
          accent: runs.length === 0 ? ACCENTS[6] : ACCENTS[1],
          href: PORTALS_PATH,
        }}
      />
      <div className="flex flex-col gap-5 lg:flex-row lg:items-start">
        <div className="min-w-0 flex-1">
          <div className="min-h-[1.2em] text-sm text-red-700">{error}</div>

          {loaded && runs.length === 0 && !error && (
            <div
              className="career-enter career-delay-1 flex flex-col items-center rounded-3xl border p-10 text-center"
              style={{ borderColor: ACCENTS[0].soft, background: `linear-gradient(160deg, #ffffff 45%, ${ACCENTS[0].soft} 100%)` }}
            >
              <span
                className="inline-flex h-14 w-14 items-center justify-center rounded-2xl text-white shadow-sm"
                style={{ background: ACCENTS[0].to }}
              >
                <PulseIcon className="h-7 w-7" />
              </span>
              <p className="mt-4 text-base font-semibold text-gray-900">No crawls are running</p>
              <p className="mt-1 text-sm text-gray-500">
                Start one from a portal link, or let auto-discovery do it. It shows up here as soon as it begins.
              </p>
            </div>
          )}

          {runs.length > 0 && (
            <div
              className="career-enter career-delay-1 relative overflow-hidden rounded-3xl border shadow-sm"
              style={{ borderColor: ACCENTS[0].soft, background: `linear-gradient(170deg, #ffffff 55%, ${ACCENTS[0].soft} 100%)` }}
            >
              <div className="relative overflow-x-auto">
                <table className="w-full text-sm">
                  <thead>
                    <tr>
                      <th className="bg-blue-50/70 p-3 text-left text-xs font-semibold uppercase tracking-wide text-blue-700">Portal</th>
                      <th className="bg-blue-50/70 p-3 text-left text-xs font-semibold uppercase tracking-wide text-blue-700">Kind</th>
                      <th className="bg-blue-50/70 p-3 text-left text-xs font-semibold uppercase tracking-wide text-blue-700">Status</th>
                      <th className="bg-blue-50/70 p-3 text-left text-xs font-semibold uppercase tracking-wide text-blue-700">Started</th>
                      <th className="bg-blue-50/70 p-3"></th>
                    </tr>
                  </thead>
                  <tbody>
                    {runs.map((run) => {
                      const kindAccent = run.kind === 'listing' ? ACCENTS[4] : ACCENTS[5]
                      return (
                        <Fragment key={run.crawlRunId}>
                          <tr className="transition-colors last:[&>td]:border-b-0 hover:bg-blue-50/50">
                            <td className="border-b border-blue-50 p-3">
                              <div className="flex items-center gap-3">
                                <span
                                  className="inline-flex h-9 w-9 shrink-0 items-center justify-center rounded-full text-sm font-bold text-white shadow-sm"
                                  style={{ background: accentFor(run.portalName).to }}
                                  aria-hidden="true"
                                >
                                  {initialOf(run.portalName)}
                                </span>
                                <div className="min-w-0">
                                  <p className="truncate font-medium">{run.portalName}</p>
                                  <a
                                    href={run.portalLinkUrl}
                                    target="_blank"
                                    rel="noreferrer"
                                    title={run.portalLinkUrl}
                                    className="block max-w-[190px] truncate text-xs text-blue-700 underline decoration-blue-200 hover:decoration-blue-500"
                                  >
                                    {run.portalLinkTitle ?? run.portalLinkUrl}
                                  </a>
                                </div>
                              </div>
                            </td>
                            <td className="border-b border-blue-50 p-3">
                              <div className="flex items-center gap-1.5">
                                <span
                                  className="inline-flex whitespace-nowrap rounded-full px-2.5 py-0.5 text-xs font-medium"
                                  style={{ background: kindAccent.soft, color: kindAccent.text }}
                                >
                                  {KIND_LABELS[run.kind]}
                                </span>
                                {run.triggeredBy === 'auto_discovery' && (
                                  <span
                                    title="Started automatically by auto-discovery, not a manual click"
                                    className="whitespace-nowrap rounded-full bg-gray-200 px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-wide text-gray-600"
                                  >
                                    Auto
                                  </span>
                                )}
                              </div>
                            </td>
                            <td className="border-b border-blue-50 p-3">
                              {run.phase === 'awaiting_human_challenge' ? (
                                <span
                                  title="Waiting for you to solve a Cloudflare challenge"
                                  className="inline-flex items-center gap-1.5 whitespace-nowrap rounded-full bg-amber-50 px-2.5 py-0.5 text-xs font-medium text-amber-800"
                                >
                                  <AlertIcon />
                                  Needs you
                                </span>
                              ) : (
                                <span className="inline-flex items-center gap-1.5 text-xs text-gray-600">
                                  <span className="h-2 w-2 shrink-0 animate-pulse rounded-full bg-green-500" aria-hidden="true" />
                                  {phaseLabel(run.phase) ?? latestCrawlLogMessage(run.log) ?? 'Running…'}
                                </span>
                              )}
                            </td>
                            <td className="border-b border-blue-50 p-3">
                              <span
                                title={new Date(run.startedAt).toLocaleString()}
                                className="whitespace-nowrap rounded-full bg-gray-100 px-2.5 py-0.5 text-xs text-gray-600"
                              >
                                {shortStarted(run.startedAt)}
                              </span>
                            </td>
                            <td className="border-b border-blue-50 p-3">
                              <div className="flex items-center justify-end gap-2">
                                <button
                                  type="button"
                                  onClick={() => {
                                    setExpandedRunId((prev) => (prev === run.crawlRunId ? null : run.crawlRunId))
                                  }}
                                  title="Show every logged step of this crawl run."
                                  className="flex items-center gap-1 whitespace-nowrap rounded-full border border-blue-200 px-3 py-1 text-xs text-blue-700 hover:bg-blue-50"
                                >
                                  <LogIcon />
                                  {expandedRunId === run.crawlRunId ? 'Hide log' : 'Log'}
                                </button>
                                <button
                                  type="button"
                                  onClick={() => {
                                    handleStop(run.portalLinkId)
                                  }}
                                  disabled={stoppingLinkId === run.portalLinkId}
                                  title="Actually stop this crawl — interrupts it on the server, not just this view."
                                  className="flex items-center gap-1 whitespace-nowrap rounded-full border border-red-200 px-3 py-1 text-xs text-red-700 hover:bg-red-50 disabled:cursor-not-allowed disabled:opacity-50"
                                >
                                  <StopIcon />
                                  {stoppingLinkId === run.portalLinkId ? 'Stopping…' : 'Stop'}
                                </button>
                              </div>
                            </td>
                          </tr>
                          {expandedRunId === run.crawlRunId && (
                            <tr>
                              <td colSpan={5} className="border-b border-blue-50 bg-gray-900 p-0">
                                <pre className="max-h-40 overflow-y-auto whitespace-pre-wrap p-3 text-xs text-gray-100">
                                  {run.log.length > 0 ? run.log.join('\n') : '(no log entries yet)'}
                                </pre>
                              </td>
                            </tr>
                          )}
                        </Fragment>
                      )
                    })}
                  </tbody>
                </table>
              </div>
            </div>
          )}
        </div>

        <aside className="w-full shrink-0 career-rail-sticky lg:w-72">
          <SummaryRail
            title="Right now"
            segments={[
              { value: listing, color: ACCENTS[4].to },
              { value: detail, color: ACCENTS[5].to },
            ]}
            centerValue={String(runs.length)}
            centerLabel={runs.length === 1 ? 'crawl running' : 'crawls running'}
            rows={[
              { label: 'Listing crawls', value: String(listing), color: ACCENTS[4].to },
              { label: 'Job detail crawls', value: String(detail), color: ACCENTS[5].to },
              { label: 'Auto-started', value: String(automatic), color: ACCENTS[6].to },
              { label: 'Needs you', value: String(waiting), color: ACCENTS[2].to, emphasis: waiting > 0 },
            ]}
          />
          <div className="mt-8">
            <PageTips page="crawlMonitor" orientation="vertical" />
          </div>
        </aside>
      </div>
    </div>
  )
}
