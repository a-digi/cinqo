import { useCallback, useEffect, useRef, useState } from 'react'
import { ApiError } from '../api/client'
import { fetchSystemEvents, type SystemEvent } from '../api/system'
import { EVENTS_POLL_MS } from './eventsRules'

// The record of unexpected stops, read when the bar appears, every EVENTS_POLL_MS while the tab is visible and on demand (after a stop was marked as
// seen). A failed read keeps what was known; a refusal (no scope) means there is nothing to show and nothing more is asked.
export function useSystemEvents(intervalMs: number = EVENTS_POLL_MS): { events: SystemEvent[]; refresh: () => void } {
  const [events, setEvents] = useState<SystemEvent[]>([])
  const refreshRef = useRef<() => void>(() => undefined)

  useEffect(() => {
    let stopped = false
    let forbidden = false
    let running = false
    let pending = false // a refresh asked for while a read was under way: read again right after it
    let timer: ReturnType<typeof setTimeout> | undefined
    let controller: AbortController | undefined

    const schedule = () => {
      if (!stopped && !forbidden) timer = setTimeout(() => void tick(), intervalMs)
    }
    const tick = async () => {
      if (running || forbidden || document.visibilityState === 'hidden') return
      running = true
      controller = new AbortController()
      try {
        const list = await fetchSystemEvents(controller.signal)
        if (!stopped) setEvents(list)
      } catch (e) {
        if (e instanceof ApiError && (e.status === 403 || e.status === 404)) forbidden = true
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
    const now = () => {
      if (running) {
        pending = true
        return
      }
      clearTimeout(timer)
      void tick()
    }
    const onVisible = () => {
      if (document.visibilityState === 'visible') now()
    }
    refreshRef.current = now
    document.addEventListener('visibilitychange', onVisible)
    void tick()
    return () => {
      stopped = true
      clearTimeout(timer)
      controller?.abort()
      document.removeEventListener('visibilitychange', onVisible)
    }
  }, [intervalMs])

  const refresh = useCallback(() => {
    refreshRef.current()
  }, [])
  return { events, refresh }
}
