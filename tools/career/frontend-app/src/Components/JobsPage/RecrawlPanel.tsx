import { AppLink } from '../../Shared/AppLink/AppLink'
import { JOB_DETAILS_PATH } from '../../paths'
import { ACCENTS } from './jobsTheme'
import type { RecrawlControl } from './useRecrawl'
import {
  detailText,
  isRunning,
  outcomeLabel,
  progressText,
  resultText,
  skippedText,
  STOP_LABEL,
  STOPPING_LABEL,
  STOPPING_TITLE,
  stoppingText,
} from './recrawlRules'

const ACCENT = ACCENTS[5] // teal

// The panel of a recrawl batch (step 148): its progress while it runs (with a Stop button), then its result, with a list of what
// happened to each job. A refused start or a batch the server no longer knows shows its message. Nothing is shown when there is
// no batch and no message.
export function RecrawlPanel({ recrawl }: { recrawl: RecrawlControl }) {
  const { status, error, startSkipped, detailsOpen } = recrawl
  if (status === null && error === '') return null

  function toggleDetails() {
    recrawl.setDetailsOpen(!detailsOpen)
  }

  const running = status !== null && isRunning(status)
  const percent = status !== null && status.total > 0 ? Math.round(((status.done + status.cancelled) / status.total) * 100) : 0
  return (
    <section
      aria-label="Recrawl"
      aria-live="polite"
      className="mb-3 rounded-2xl border p-4"
      style={{ borderColor: ACCENT.soft, background: `linear-gradient(160deg, #ffffff 40%, ${ACCENT.soft} 100%)` }}
    >
      {error !== '' && (
        <div className="flex items-start justify-between gap-3">
          <p role="alert" className="text-sm text-red-700">
            {error}
          </p>
          <button type="button" onClick={recrawl.dismiss} className="shrink-0 text-xs text-gray-500 underline hover:text-gray-700">
            Close
          </button>
        </div>
      )}
      {status !== null && (
        <>
          <div className="flex flex-wrap items-center justify-between gap-2">
            <p className="text-sm font-semibold" style={{ color: ACCENT.text }}>
              {running ? (recrawl.stopping ? stoppingText(status) : progressText(status)) : resultText(status, startSkipped.length)}
            </p>
            <div className="flex items-center gap-2">
              {running && (
                <button
                  type="button"
                  disabled={recrawl.stopping}
                  title={recrawl.stopping ? STOPPING_TITLE : undefined}
                  onClick={() => {
                    void recrawl.stop()
                  }}
                  className="rounded-md border border-gray-300 bg-white/70 px-3 py-1 text-xs font-medium text-gray-700 hover:bg-white disabled:cursor-not-allowed disabled:opacity-50 disabled:hover:bg-white/70"
                >
                  {recrawl.stopping ? STOPPING_LABEL : STOP_LABEL}
                </button>
              )}
              <button type="button" onClick={toggleDetails} className="text-xs text-gray-600 underline hover:text-gray-800">
                {detailsOpen ? 'Hide details' : 'Details'}
              </button>
              {!running && (
                <button type="button" onClick={recrawl.dismiss} className="text-xs text-gray-600 underline hover:text-gray-800">
                  Close
                </button>
              )}
            </div>
          </div>
          {running && (
            <div className="mt-2 h-1.5 overflow-hidden rounded-full bg-white/70" aria-hidden="true">
              <div className="h-full rounded-full" style={{ width: `${String(percent)}%`, background: ACCENT.to }} />
            </div>
          )}
          {detailsOpen && (
            <div className="mt-3 border-t border-white pt-3">
              {startSkipped.length > 0 && <p className="mb-2 text-xs text-gray-600">{skippedText(startSkipped)}</p>}
              {status.jobs.length === 0 ? (
                <p className="text-xs text-gray-500">{running ? 'Loading…' : 'No jobs to list.'}</p>
              ) : (
                <ul className="space-y-1.5">
                  {status.jobs.map((j) => {
                    const detail = detailText(j)
                    return (
                      <li key={j.jobId} className="flex flex-wrap items-baseline justify-between gap-x-3 text-xs">
                        <span className="min-w-0">
                          <AppLink
                            href={`${JOB_DETAILS_PATH}?id=${encodeURIComponent(j.jobId)}`}
                            className="font-medium text-gray-800 hover:underline"
                          >
                            {j.title}
                          </AppLink>
                          {j.manual && (
                            <span className="ml-1.5 rounded-full bg-white/80 px-1.5 py-0.5 text-[10px] font-medium text-gray-600">
                              Added by hand
                            </span>
                          )}
                          {detail !== '' && <span className="block text-gray-500">{detail}</span>}
                        </span>
                        <span className="shrink-0 font-medium text-gray-700">{outcomeLabel(j)}</span>
                      </li>
                    )
                  })}
                </ul>
              )}
              {status.total > status.jobs.length && status.jobs.length > 0 && (
                <p className="mt-2 text-xs text-gray-500">{`Showing the first ${String(status.jobs.length)} of ${String(status.total)} jobs.`}</p>
              )}
            </div>
          )}
        </>
      )}
    </section>
  )
}
