import { useCallback, useEffect, useRef, useState } from 'react'
import { fetchToolUpdates, startUpdateCheck, type ToolUpdatesResponse } from '../../../api/toolUpdates'
import { errorText } from './repositoryErrors'
import { POLL_INTERVAL_MS, gaveUpPolling, shouldKeepPolling } from './updateCheckDisplay'

// The Tools page's view of the update check: asks the server when the page opens
// and, while a check is running (the one started at launch, or the button's),
// asks again every 1.5 s until it is done, the page is left, or 3 minutes passed.
export function useUpdateCheck() {
  const [data, setData] = useState<ToolUpdatesResponse | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [gaveUp, setGaveUp] = useState(false)
  const [starting, setStarting] = useState(false)

  const alive = useRef(true)
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null)
  const pollStart = useRef(0)

  const clearTimer = () => {
    if (timer.current !== null) clearTimeout(timer.current)
    timer.current = null
  }

  const apply = useCallback((next: ToolUpdatesResponse, schedule: () => void) => {
    if (!alive.current) return
    setData(next)
    setError(null)
    const elapsed = Date.now() - pollStart.current
    setGaveUp(gaveUpPolling(next.check, elapsed))
    if (shouldKeepPolling(next.check, elapsed)) {
      clearTimer()
      timer.current = setTimeout(schedule, POLL_INTERVAL_MS)
    }
  }, [])

  const poll = useCallback(async () => {
    try {
      const next = await fetchToolUpdates()
      apply(next, () => {
        void poll()
      })
    } catch (err) {
      if (alive.current) setError(errorText(err, 'Failed to check for updates.'))
    }
  }, [apply])

  useEffect(() => {
    alive.current = true
    pollStart.current = Date.now()
    void poll()
    return () => {
      alive.current = false
      clearTimer()
    }
  }, [poll])

  // The button: start a check (or join the running one) and follow it.
  const start = useCallback(async () => {
    setStarting(true)
    setError(null)
    setGaveUp(false)
    clearTimer()
    pollStart.current = Date.now()
    try {
      const next = await startUpdateCheck()
      apply(next, () => {
        void poll()
      })
    } catch (err) {
      if (alive.current) setError(errorText(err, 'Failed to start the update check.'))
    } finally {
      if (alive.current) setStarting(false)
    }
  }, [apply, poll])

  return { data, error, gaveUp, starting, start }
}
