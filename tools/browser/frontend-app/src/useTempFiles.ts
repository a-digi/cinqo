import { useCallback, useEffect, useState } from 'react'
import { fetchTempFiles, type TempFilesResponse } from './api'

export const TEMP_FILES_POLL_MS = 10000

// loading: nothing answered yet; missing: the backend has no such route (an older Browser tool); error: the last call failed.
export type TempFilesState =
  { kind: 'loading' } | { kind: 'missing' } | { kind: 'error'; message: string } | { kind: 'ready'; data: TempFilesResponse }

// Reads the temporary files folder when the page opens, every TEMP_FILES_POLL_MS while the tab is visible (a hidden tab does not ask and asks at once when
// shown again), and whenever refresh() is called (after a delete).
export function useTempFiles(intervalMs: number = TEMP_FILES_POLL_MS): { state: TempFilesState; refresh: () => void } {
  const [state, setState] = useState<TempFilesState>({ kind: 'loading' })
  const [version, setVersion] = useState(0)
  const refresh = useCallback(() => {
    setVersion((v) => v + 1)
  }, [])

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
        const data = await fetchTempFiles(controller.signal)
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
  }, [intervalMs, version])

  return { state, refresh }
}
