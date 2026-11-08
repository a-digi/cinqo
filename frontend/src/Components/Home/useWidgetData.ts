import { useCallback, useEffect, useState } from 'react'
import { fetchWidgetData, type WidgetData } from '../../api/dashboard'
import { nextDelayMs } from './widgetDisplay'

export type WidgetResult = { status: 'loading' } | { status: 'failed' } | { status: 'ready'; data: WidgetData }

// Fetches one widget's data, then keeps it fresh: it asks again after the widget's
// own refresh interval (sooner after a failure) and never while the tab is hidden —
// a refresh that came due in the background runs as soon as the tab is visible.
// A request that fails keeps showing the last good data instead of blanking it.
export function useWidgetData(id: string, refreshSeconds: number): { result: WidgetResult; retry: () => void } {
  const [result, setResult] = useState<WidgetResult>({ status: 'loading' })
  const [attempt, setAttempt] = useState(0)

  useEffect(() => {
    let cancelled = false
    let timer: number | undefined
    let due = false

    const schedule = (ms: number) => {
      timer = window.setTimeout(() => {
        if (document.hidden) due = true
        else void run()
      }, ms)
    }

    const run = async () => {
      due = false
      try {
        const data = await fetchWidgetData(id)
        if (cancelled) return
        setResult({ status: 'ready', data })
        schedule(nextDelayMs(refreshSeconds, data.state))
      } catch {
        if (cancelled) return
        setResult((prev) => (prev.status === 'ready' ? prev : { status: 'failed' }))
        schedule(nextDelayMs(refreshSeconds, 'request-failed'))
      }
    }

    const onVisible = () => {
      if (!document.hidden && due) void run()
    }

    document.addEventListener('visibilitychange', onVisible)
    void run()
    return () => {
      cancelled = true
      window.clearTimeout(timer)
      document.removeEventListener('visibilitychange', onVisible)
    }
  }, [id, refreshSeconds, attempt])

  const retry = useCallback(() => {
    setResult({ status: 'loading' })
    setAttempt((n) => n + 1)
  }, [])

  return { result, retry }
}
