import { useCallback, useEffect, useRef, useState } from 'react'
import { followLoop, type LoopState } from './followLoop'
import { MAX_HELD, mergeBatch, newTail, type TailState } from './tailRules'
import type { LiveFeed } from './types'

function sleep(ms: number, signal: AbortSignal): Promise<void> {
  return new Promise((resolve) => {
    const timer = window.setTimeout(resolve, ms)
    signal.addEventListener(
      'abort',
      () => {
        window.clearTimeout(timer)
        resolve()
      },
      { once: true },
    )
  })
}

// Resolves when the tab becomes visible (or the signal aborts): a hidden tab does not poll.
function waitVisible(signal: AbortSignal): Promise<void> {
  return new Promise((resolve) => {
    const done = () => {
      document.removeEventListener('visibilitychange', onChange)
      signal.removeEventListener('abort', done)
      resolve()
    }
    const onChange = () => {
      if (document.visibilityState === 'visible') done()
    }
    document.addEventListener('visibilitychange', onChange)
    signal.addEventListener('abort', done, { once: true })
  })
}

export interface LiveFeedState {
  tail: TailState
  loop: LoopState
  paused: boolean
  pause: () => void
  // Continues from where it stopped: what was written meanwhile arrives (up to a batch at a time).
  resume: () => void
  // Forgets what is held and starts at the end of the log again.
  restart: () => void
}

// Follows a feed: polls it (never two at once, slower while quiet, not at all while the tab is hidden or paused), keeps the last lines in a
// bounded buffer, and reports what the loop is doing. A null feed follows nothing. A new feed starts afresh.
export function useLiveFeed(feed: LiveFeed | null, maxHeld: number = MAX_HELD): LiveFeedState {
  const [tail, setTail] = useState<TailState>(newTail)
  const [loop, setLoop] = useState<LoopState>({ kind: 'following' })
  const [paused, setPaused] = useState(false)
  const [round, setRound] = useState(0)
  const cursor = useRef<string | undefined>(undefined)

  // A different feed, or a restart, starts from the end of the log with nothing held.
  useEffect(() => {
    cursor.current = undefined
    setTail(newTail())
    setLoop({ kind: 'following' })
    setPaused(false)
  }, [feed, round])

  useEffect(() => {
    if (!feed || paused) return
    const ctl = new AbortController()
    void followLoop({
      feed,
      signal: ctl.signal,
      startCursor: cursor.current,
      apply: (batch) => {
        setTail((state) => mergeBatch(state, batch, maxHeld))
      },
      onCursor: (next) => {
        cursor.current = next
      },
      onState: setLoop,
      sleep,
      waitVisible,
      isVisible: () => document.visibilityState === 'visible',
    })
    return () => {
      ctl.abort()
    }
  }, [feed, paused, round, maxHeld])

  const pause = useCallback(() => {
    setPaused(true)
  }, [])
  const resume = useCallback(() => {
    setPaused(false)
  }, [])
  const restart = useCallback(() => {
    setRound((n) => n + 1)
  }, [])
  return { tail, loop, paused, pause, resume, restart }
}
