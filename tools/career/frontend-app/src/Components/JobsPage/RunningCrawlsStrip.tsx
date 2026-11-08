import { useEffect, useState } from 'react'
import type { ActiveCrawlRunSummary } from '../../Crawler/crawlNow'
import { CRAWL_MONITOR_PATH } from '../../paths'
import { headline, runLine } from './runningCrawlsRules'

// "A crawler is running" (plan 214): shown under the page header only while a crawl runs, so a person on the Jobs page sees that jobs or descriptions are
// being collected. One line per run; the log is left to the Crawl Monitor, which the link opens.
export function RunningCrawlsStrip({ runs }: { runs: ActiveCrawlRunSummary[] }) {
  const [now, setNow] = useState(() => new Date())
  const running = runs.length > 0
  useEffect(() => {
    if (!running) return
    const timer = setInterval(() => {
      setNow(new Date())
    }, 1000)
    return () => {
      clearInterval(timer)
    }
  }, [running])
  if (!running) return null
  return (
    <section
      role="status"
      aria-live="polite"
      data-testid="running-crawls"
      className="mb-4 rounded-xl border border-blue-200 bg-blue-50 px-4 py-3 text-sm text-blue-900"
    >
      <div className="flex flex-wrap items-center justify-between gap-2">
        <p className="flex items-center gap-2 font-medium">
          <span className="inline-flex gap-1" aria-hidden="true">
            <span className="h-2 w-2 animate-bounce rounded-full bg-blue-500 [animation-delay:-0.3s]" />
            <span className="h-2 w-2 animate-bounce rounded-full bg-blue-500 [animation-delay:-0.15s]" />
            <span className="h-2 w-2 animate-bounce rounded-full bg-blue-500" />
          </span>
          {headline(runs.length)}
        </p>
        <button
          type="button"
          onClick={() => {
            window.__cinqoToolBridge.navigate(CRAWL_MONITOR_PATH)
          }}
          className="text-xs font-medium underline hover:text-blue-950"
        >
          Open Crawl Monitor
        </button>
      </div>
      <ul className="mt-2 space-y-1">
        {runs.map((run) => {
          const line = runLine(run, now)
          return (
            <li key={line.key} className="flex flex-wrap items-baseline gap-x-2 text-xs">
              <span className="font-medium">{line.where}</span>
              <span>{line.what}</span>
              {line.phase !== '' && <span className="text-blue-700">· {line.phase}</span>}
              {line.automatic && <span className="text-blue-700">· started automatically</span>}
              {line.elapsed !== '' && <span className="tabular-nums text-blue-700">· {line.elapsed}</span>}
            </li>
          )
        })}
      </ul>
    </section>
  )
}
