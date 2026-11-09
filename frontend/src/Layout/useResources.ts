import { useEffect, useState } from 'react'
import { ApiError } from '../api/client'
import { fetchSystemResources, type SystemResources } from '../api/system'

// loading: nothing answered yet. hidden: this person may not see it (no scope), so there is no bar. unknown: the call failed.
export type ResourcesState = { kind: 'loading' } | { kind: 'hidden' } | { kind: 'unknown' } | { kind: 'ready'; resources: SystemResources }

export const POLL_MS = 2000

// Asks every mounted bar to read the resources now (after an unload, so the chip does not wait for the next poll).
export const REFRESH_EVENT = 'cinqo:resources-refresh'
export function refreshResources(): void {
  window.dispatchEvent(new Event(REFRESH_EVENT))
}

// Reads the resources every POLL_MS while the tab is visible; a hidden tab does not ask, and asks at once when it is shown again.
export function useResources(intervalMs: number = POLL_MS): ResourcesState {
  const [state, setState] = useState<ResourcesState>({ kind: 'loading' })

  useEffect(() => {
    let stopped = false
    let timer: ReturnType<typeof setTimeout> | undefined
    let controller: AbortController | undefined
    let running = false
    let pending = false // a refresh asked for while a read was under way: read again right after it

    const schedule = () => {
      if (stopped) return
      timer = setTimeout(() => void tick(), intervalMs)
    }
    const tick = async () => {
      if (running || document.visibilityState === 'hidden') return // a hidden tab is asked again by onVisible
      running = true
      controller = new AbortController()
      try {
        const resources = await fetchSystemResources(controller.signal)
        if (stopped) return
        setState({ kind: 'ready', resources })
      } catch (e) {
        if (stopped) return
        if (e instanceof ApiError && e.status === 403) {
          setState({ kind: 'hidden' })
          return // no scope: never ask again
        }
        setState({ kind: 'unknown' })
      } finally {
        running = false
      }
      if (pending) {
        pending = false
        timer = setTimeout(() => void tick(), 0)
        return
      }
      schedule()
    }
    const onVisible = () => {
      if (document.visibilityState !== 'visible') return
      clearTimeout(timer)
      void tick()
    }

    const onRefresh = () => {
      if (running) {
        pending = true
        return
      }
      clearTimeout(timer)
      void tick()
    }

    document.addEventListener('visibilitychange', onVisible)
    window.addEventListener(REFRESH_EVENT, onRefresh)
    void tick()
    return () => {
      stopped = true
      clearTimeout(timer)
      controller?.abort()
      document.removeEventListener('visibilitychange', onVisible)
      window.removeEventListener(REFRESH_EVENT, onRefresh)
    }
  }, [intervalMs])

  return state
}
