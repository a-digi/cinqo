import { useEffect, useState } from 'react'
import { fetchMemoryTop, type MemoryTop } from '../api/system'
import { TOP_POLL_MS } from './topRules'

export type MemoryTopState = { kind: 'loading' } | { kind: 'error'; message: string } | { kind: 'ready'; data: MemoryTop }

// Reads the top applications at once when `open` becomes true and then every TOP_POLL_MS while it stays open and the tab is visible; closed, it
// asks nothing. `retry` reads again now (after an error).
export function useMemoryTop(open: boolean): { state: MemoryTopState; retry: () => void } {
  const [state, setState] = useState<MemoryTopState>({ kind: 'loading' })
  const [nonce, setNonce] = useState(0)

  useEffect(() => {
    if (!open) return
    let stopped = false
    let running = false
    let timer: ReturnType<typeof setTimeout> | undefined
    let controller: AbortController | undefined

    const schedule = () => {
      if (!stopped) timer = setTimeout(() => void tick(), TOP_POLL_MS)
    }
    const tick = async () => {
      if (running || document.visibilityState === 'hidden') return
      running = true
      controller = new AbortController()
      try {
        const data = await fetchMemoryTop(controller.signal)
        if (stopped) return
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
    setState((s) => (s.kind === 'ready' ? s : { kind: 'loading' }))
    document.addEventListener('visibilitychange', onVisible)
    void tick()
    return () => {
      stopped = true
      clearTimeout(timer)
      controller?.abort()
      document.removeEventListener('visibilitychange', onVisible)
    }
  }, [open, nonce])

  return {
    state,
    retry: () => {
      setNonce((n) => n + 1)
    },
  }
}
