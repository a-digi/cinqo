import { useEffect, useState, type ReactNode } from 'react'
import { fetchCrawlEngine, type EngineReading } from './crawlEngine'
import { CrawlEngineContext } from './useCrawlEngine'

// Asked once when the page opens and again whenever the tab becomes visible, so switching the engine on the Browser & Engine page shows in
// the status and the instructions without a reload.
export function CrawlEngineProvider({ children }: { children: ReactNode }) {
  const [engine, setEngine] = useState<EngineReading>(null)

  useEffect(() => {
    let cancelled = false
    let controller: AbortController | null = null
    const ask = () => {
      controller?.abort()
      controller = new AbortController()
      fetchCrawlEngine(controller.signal)
        .then((e) => {
          if (!cancelled) setEngine(e)
        })
        .catch(() => {
          // Could not be read: keep what was known (or nothing).
        })
    }
    const onVisible = () => {
      if (document.visibilityState === 'visible') ask()
    }
    ask()
    document.addEventListener('visibilitychange', onVisible)
    return () => {
      cancelled = true
      controller?.abort()
      document.removeEventListener('visibilitychange', onVisible)
    }
  }, [])

  return <CrawlEngineContext.Provider value={engine}>{children}</CrawlEngineContext.Provider>
}
