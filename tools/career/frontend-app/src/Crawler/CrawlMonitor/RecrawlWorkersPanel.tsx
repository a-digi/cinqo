import { useState } from 'react'
import { cancelRecrawl, type RecrawlActive } from '../../api'
import { ACCENTS, accentFor, initialOf } from '../../Components/JobsPage/jobsTheme'
import { STOP_LABEL, STOPPING_LABEL, STOPPING_TITLE } from '../../Components/JobsPage/recrawlRules'
import { AppLink } from '../../Shared/AppLink/AppLink'
import { JOB_DETAILS_PATH } from '../../paths'
import { AlertIcon, StopIcon } from '../../Shared/Icons/icons'
import {
  batchLine,
  batchPercent,
  crawlerText,
  elapsedText,
  needsYou,
  WAITING_BETWEEN_JOBS,
  workersHeading,
  workerPhaseText,
} from './recrawlMonitorRules'

const ACCENT = ACCENTS[5] // teal, the colour of the job-detail kind

// The "Job detail recrawl" part of the Crawl Monitor (plan 194): the batches of the Jobs page's "Recrawl" and "Crawl missing details" with
// their progress and a Stop button, and one row for each job that is being crawled right now (up to five at a time), with the Browser's
// phase and how long it has been going. A recrawl has no crawl run, so the table of crawl runs above never showed it.
export function RecrawlWorkersPanel({ active, now, onChanged }: { active: RecrawlActive; now: number; onChanged: () => void }) {
  // Batches whose Stop was clicked here: the button waits for the answer, then follows the server's own "stopping" flag.
  const [stopping, setStopping] = useState<ReadonlySet<string>>(new Set())
  const [error, setError] = useState('')

  if (active.batches.length === 0 && active.workers.length === 0) return null

  function stop(batchId: string) {
    setError('')
    setStopping((prev) => new Set(prev).add(batchId))
    cancelRecrawl(batchId)
      .then(onChanged)
      .catch((err: unknown) => {
        setStopping((prev) => {
          const next = new Set(prev)
          next.delete(batchId)
          return next
        })
        setError(err instanceof Error ? err.message : 'Failed to stop the recrawl.')
      })
  }

  return (
    <section
      aria-label="Job detail recrawl"
      className="career-enter career-delay-2 mt-5 overflow-hidden rounded-3xl border shadow-sm"
      style={{ borderColor: ACCENT.soft, background: `linear-gradient(170deg, #ffffff 55%, ${ACCENT.soft} 100%)` }}
    >
      <div className="border-b p-4" style={{ borderColor: ACCENT.soft }}>
        <h2 className="text-xs font-semibold uppercase tracking-wide" style={{ color: ACCENT.text }}>
          Job detail recrawl
        </h2>
        {error !== '' && (
          <p role="alert" className="mt-2 text-sm text-red-700">
            {error}
          </p>
        )}
        {active.batches.map((b) => {
          const isStopping = b.stopping || stopping.has(b.batchId)
          return (
            <div key={b.batchId} className="mt-3 first:mt-2" data-testid="recrawl-batch">
              <div className="flex flex-wrap items-center justify-between gap-2">
                <p role="status" className="text-sm font-semibold" style={{ color: ACCENT.text }}>
                  {batchLine(isStopping && !b.stopping ? { ...b, stopping: true } : b)}
                </p>
                <button
                  type="button"
                  disabled={isStopping}
                  title={isStopping ? STOPPING_TITLE : 'Cancel the jobs that wait. The ones being crawled right now finish first.'}
                  onClick={() => {
                    stop(b.batchId)
                  }}
                  className="flex items-center gap-1 whitespace-nowrap rounded-full border border-red-200 px-3 py-1 text-xs text-red-700 hover:bg-red-50 disabled:cursor-not-allowed disabled:opacity-50 disabled:hover:bg-transparent"
                >
                  <StopIcon />
                  {isStopping ? STOPPING_LABEL : STOP_LABEL}
                </button>
              </div>
              <div className="mt-2 h-1.5 overflow-hidden rounded-full bg-gray-200" aria-hidden="true">
                <div
                  className="h-full rounded-full transition-all"
                  style={{ width: `${String(batchPercent(b))}%`, background: ACCENT.to }}
                />
              </div>
            </div>
          )
        })}
      </div>

      {active.workers.length === 0 ? (
        <p className="p-4 text-sm text-gray-500">{WAITING_BETWEEN_JOBS}</p>
      ) : (
        <div className="relative overflow-x-auto">
          <p className="px-4 pt-3 text-xs text-gray-500">{workersHeading(active.workers.length)}</p>
          <table className="w-full text-sm">
            <thead>
              <tr>
                {['Platform', 'Job', 'Status', 'Running for', 'Crawler'].map((h) => (
                  <th key={h} className="p-3 text-left text-xs font-semibold uppercase tracking-wide" style={{ color: ACCENT.text }}>
                    {h}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {active.workers.map((w) => {
                const platform = w.portalName ?? ''
                return (
                  <tr key={w.jobId} data-testid="recrawl-worker" className="transition-colors last:[&>td]:border-b-0 hover:bg-teal-50/50">
                    <td className="border-b border-teal-50 p-3">
                      <div className="flex items-center gap-3">
                        <span
                          className="inline-flex h-8 w-8 shrink-0 items-center justify-center rounded-full text-xs font-bold text-white shadow-sm"
                          style={{ background: accentFor(platform || w.jobTitle).to }}
                          aria-hidden="true"
                        >
                          {initialOf(platform || w.jobTitle)}
                        </span>
                        <div className="min-w-0">
                          <p className="truncate font-medium">{platform || '—'}</p>
                          {w.portalLinkTitle !== undefined && w.portalLinkTitle !== '' && (
                            <p className="max-w-[160px] truncate text-xs text-gray-500" title={w.portalLinkTitle}>
                              {w.portalLinkTitle}
                            </p>
                          )}
                        </div>
                      </div>
                    </td>
                    <td className="border-b border-teal-50 p-3">
                      <span className="block max-w-[320px] truncate" title={w.url}>
                        <AppLink
                          href={`${JOB_DETAILS_PATH}?id=${encodeURIComponent(w.jobId)}`}
                          className="text-blue-700 underline decoration-blue-200 hover:decoration-blue-500"
                        >
                          {w.jobTitle || w.url}
                        </AppLink>
                      </span>
                    </td>
                    <td className="border-b border-teal-50 p-3">
                      {needsYou(w) ? (
                        <span className="inline-flex items-center gap-1.5 whitespace-nowrap rounded-full bg-amber-50 px-2.5 py-0.5 text-xs font-medium text-amber-800">
                          <AlertIcon />
                          Needs you
                        </span>
                      ) : (
                        <span className="inline-flex items-center gap-1.5 text-xs text-gray-600" title={w.message ?? ''}>
                          <span
                            className={`h-2 w-2 shrink-0 rounded-full ${w.phase ? 'animate-pulse bg-green-500' : 'bg-blue-300'}`}
                            aria-hidden="true"
                          />
                          {workerPhaseText(w)}
                        </span>
                      )}
                    </td>
                    <td className="border-b border-teal-50 p-3">
                      <span
                        title={new Date(w.startedAt).toLocaleString()}
                        className="whitespace-nowrap rounded-full bg-gray-100 px-2.5 py-0.5 text-xs text-gray-600"
                      >
                        {elapsedText(w.startedAt, now)}
                      </span>
                    </td>
                    <td className="border-b border-teal-50 p-3 text-xs text-gray-500">{crawlerText(w.slot)}</td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
      )}
    </section>
  )
}
