import { useEffect, useState } from 'react'
import { fetchLiveProviders } from './liveLogsApi'
import type { LiveProvider } from './types'

// The providers the caller may follow: null while loading, an empty list when there are none or the server cannot say (a core without the
// module, a failure): the live views then simply do not appear, which is also what removing the module leaves behind.
export function useLiveProviders(): LiveProvider[] | null {
  const [providers, setProviders] = useState<LiveProvider[] | null>(null)
  useEffect(() => {
    const ctl = new AbortController()
    fetchLiveProviders(ctl.signal)
      .then((list) => {
        if (!ctl.signal.aborted) setProviders(list)
      })
      .catch(() => {
        if (!ctl.signal.aborted) setProviders([])
      })
    return () => {
      ctl.abort()
    }
  }, [])
  return providers
}
