import { useEffect, useState } from 'react'
import { fetchChromeProcesses, type ChromeProcessesResponse } from './api'

export const CHROME_POLL_MS = 3000

// loading: nothing answered yet; missing: the backend has no such route (an older Browser tool); error: the last call failed.
export type ChromeProcessesState =
  { kind: 'loading' } | { kind: 'missing' } | { kind: 'error'; message: string } | { kind: 'ready'; data: ChromeProcessesResponse }

// Reads the Chrome sessions every CHROME_POLL_MS while the tab is visible (a hidden tab does not ask and asks at once when shown again).
export function useChromeProcesses(intervalMs: number = CHROME_POLL_MS): ChromeProcessesState {
  const [state, setState] = useState<ChromeProcessesState>({ kind: 'loading' })

  useEffect(() => {
    let stopped = false
    let running = false
    let timer: ReturnType<typeof setTimeout> | undefined
    let controller: AbortController | undefined

    const schedule = () => {
      if (!stopped) timer = setTimeout(() => void tick(), intervalMs)
    }
    const tick = async () => {
      if (running || document.visibilityState === 'hidden') return
      running = true
      controller = new AbortController()
      try {
        const data = await fetchChromeProcesses(controller.signal)
        if (stopped) return
        if (data === null) {
          setState({ kind: 'missing' })
          return // an older backend: never ask again
        }
        setState({ kind: 'ready', data })
      } catch (e) {
        if (stopped) return
        setState({ kind: 'error', message: e instanceof Error ? e.message : String(e) })
      } finally {
        running = false
      }
      schedule()
    }
    const onVisible = () => {
      if (document.visibilityState !== 'visible') return
      clearTimeout(timer)
      void tick()
    }

    document.addEventListener('visibilitychange', onVisible)
    void tick()
    return () => {
      stopped = true
      clearTimeout(timer)
      controller?.abort()
      document.removeEventListener('visibilitychange', onVisible)
    }
  }, [intervalMs])

  return state
}
