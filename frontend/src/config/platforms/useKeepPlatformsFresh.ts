import { useEffect } from 'react'
import { usePlatforms } from './PlatformsContext'
import { POLL_INTERVAL_MS } from './platformsRules'

// While the component that calls it is on screen, the platforms are read every POLL_INTERVAL_MS, but only while the tab is visible (a hidden tab reads
// nothing; coming back reads at once, see PlatformsProvider). The chat's "no AI is available" view calls it: it is on screen exactly until a platform can
// be used, so it fixes itself within seconds of a key or a model being added, and the polling stops with it (plan 243, step 3).
export function useKeepPlatformsFresh(): void {
  const { refresh } = usePlatforms()
  useEffect(() => {
    const timer = window.setInterval(() => {
      if (document.visibilityState === 'visible') void refresh()
    }, POLL_INTERVAL_MS)
    return () => {
      window.clearInterval(timer)
    }
  }, [refresh])
}
