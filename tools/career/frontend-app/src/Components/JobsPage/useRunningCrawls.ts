import { useEffect, useRef, useState } from 'react'
import { fetchActiveCrawlRuns, type ActiveCrawlRunSummary } from '../../Crawler/crawlNow'
import { someRunEnded } from './runningCrawlsRules'

// How often the running crawls are asked for (plan 214), while the tab is visible. A push from the server (plan 193) would replace this.
export const RUNNING_CRAWLS_POLL_MS = 4000

// The crawls running right now (the Crawl Monitor's own data), kept fresh while the page is open. onEnded is called once when a run that was
// there before is gone (the page reloads its jobs, new ones or new descriptions have arrived). A failed ask keeps what was known.
export function useRunningCrawls(onEnded: () => void): ActiveCrawlRunSummary[] {
  const [runs, setRuns] = useState<ActiveCrawlRunSummary[]>([])
  const previous = useRef<ActiveCrawlRunSummary[]>([])
  const ended = useRef(onEnded)
  ended.current = onEnded

  useEffect(() => {
    let cancelled = false
    function ask() {
      if (document.visibilityState === 'hidden') return
      fetchActiveCrawlRuns()
        .then((next) => {
          if (cancelled) return
          const gone = someRunEnded(previous.current, next)
          previous.current = next
          setRuns(next)
          if (gone) ended.current()
        })
        .catch(() => undefined)
    }
    ask()
    const timer = setInterval(ask, RUNNING_CRAWLS_POLL_MS)
    document.addEventListener('visibilitychange', ask)
    return () => {
      cancelled = true
      clearInterval(timer)
      document.removeEventListener('visibilitychange', ask)
    }
  }, [])

  return runs
}
