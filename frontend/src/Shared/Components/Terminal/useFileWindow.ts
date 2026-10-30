import { useCallback, useEffect, useMemo, useState } from 'react'
import { ApiError } from '../../../api/client'
import { fetchFileInfo, fetchFileLines, type FileInfo, type FileLine } from '../../../api/fileReader'
import { clampFrom, describeFailure, fromFraction, jumpFrom, lastFrom, pageFrom } from './terminalRules'

// How long to wait between asking the server to index further, and how many times a window waits for the
// index to reach it (each call also advances the index by about 750 ms on the server).
const POLL_MS = 250
const WAIT_MS = 400
const MAX_WAITS = 150

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

function failure(err: unknown): string {
  return describeFailure(err instanceof ApiError ? err.status : undefined)
}

export interface FileWindowState {
  info: FileInfo | null
  lines: FileLine[]
  // The first line of the lines shown.
  shownFrom: number
  // The line the person asked for with "go to line" (until they move elsewhere), so it can be marked.
  target: number | null
  loading: boolean
  error: string | null
  goTo: (line: number) => void
  page: (direction: 1 | -1) => void
  top: () => void
  bottom: () => void
  moveToFraction: (fraction: number) => void
  retry: () => void
}

// Fetches a file's info and one window of lines at a time from the file reader. The server indexes a big
// file in slices, so the info is asked for repeatedly until it is complete, and a window beyond what is
// indexed so far waits for it instead of failing. A window that is being replaced stays on screen until its
// successor arrives.
export function useFileWindow(source: string, fileRef: string, windowSize: number): FileWindowState {
  const [info, setInfo] = useState<FileInfo | null>(null)
  const [win, setWin] = useState<{ from: number; lines: FileLine[] } | null>(null)
  const [from, setFrom] = useState(1)
  const [wantBottom, setWantBottom] = useState(false)
  const [target, setTarget] = useState<number | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [attempt, setAttempt] = useState(0)

  const complete = info?.complete ?? false
  const total = info?.totalLines ?? 0

  // Until the index is complete the total is only a lower bound, so no upper clamp is applied then.
  const start = useMemo(() => (complete ? clampFrom(from, total, windowSize) : Math.max(1, from)), [complete, from, total, windowSize])

  // A new file starts again from the top.
  useEffect(() => {
    setInfo(null)
    setWin(null)
    setFrom(1)
    setWantBottom(false)
    setTarget(null)
    setError(null)
    setLoading(true)
  }, [source, fileRef, attempt])

  // Keep asking for the info until the whole file is indexed.
  useEffect(() => {
    const ctl = new AbortController()
    // a function, so the checks after each await are not narrowed to "never aborted"
    const cancelled = () => ctl.signal.aborted
    void (async () => {
      try {
        for (;;) {
          const next = await fetchFileInfo(source, fileRef, ctl.signal)
          if (cancelled()) return
          setInfo(next)
          if (next.complete) return
          await sleep(POLL_MS, ctl.signal)
          if (cancelled()) return
        }
      } catch (err) {
        if (!cancelled()) setError(failure(err))
      }
    })()
    return () => {
      ctl.abort()
    }
  }, [source, fileRef, attempt])

  // The window itself.
  useEffect(() => {
    const ctl = new AbortController()
    setLoading(true)
    void (async () => {
      try {
        for (let waits = 0; ; waits++) {
          try {
            const next = await fetchFileLines(source, fileRef, start, windowSize, ctl.signal)
            if (ctl.signal.aborted) return
            setWin({ from: next.from, lines: next.lines })
            setError(null)
            setLoading(false)
            return
          } catch (err) {
            // 409: the index has not reached that far yet. Wait for it.
            if (err instanceof ApiError && err.status === 409 && waits < MAX_WAITS) {
              await sleep(WAIT_MS, ctl.signal)
              if (ctl.signal.aborted) return
              continue
            }
            throw err
          }
        }
      } catch (err) {
        if (ctl.signal.aborted) return
        setError(failure(err))
        setLoading(false)
      }
    })()
    return () => {
      ctl.abort()
    }
  }, [source, fileRef, start, windowSize, attempt])

  // "End" asked before the file was fully indexed: go there once it is.
  useEffect(() => {
    if (wantBottom && complete) {
      setFrom(lastFrom(total, windowSize))
      setWantBottom(false)
    }
  }, [wantBottom, complete, total, windowSize])

  const goTo = useCallback(
    (line: number) => {
      setWantBottom(false)
      setTarget(complete ? Math.min(line, total) : line)
      setFrom(complete ? jumpFrom(line, total, windowSize) : Math.max(1, line))
    },
    [complete, total, windowSize],
  )
  const page = useCallback(
    (direction: 1 | -1) => {
      setWantBottom(false)
      setTarget(null)
      setFrom(complete ? pageFrom(start, direction, total, windowSize) : Math.max(1, start + direction * windowSize))
    },
    [complete, start, total, windowSize],
  )
  const top = useCallback(() => {
    setWantBottom(false)
    setTarget(null)
    setFrom(1)
  }, [])
  const bottom = useCallback(() => {
    setTarget(null)
    if (complete) setFrom(lastFrom(total, windowSize))
    else setWantBottom(true)
  }, [complete, total, windowSize])
  const moveToFraction = useCallback(
    (fraction: number) => {
      if (!complete) return
      setWantBottom(false)
      setTarget(null)
      setFrom(fromFraction(fraction, total, windowSize))
    },
    [complete, total, windowSize],
  )
  const retry = useCallback(() => {
    setAttempt((n) => n + 1)
  }, [])

  return {
    info,
    lines: win?.lines ?? [],
    shownFrom: win?.from ?? 1,
    target,
    loading,
    error,
    goTo,
    page,
    top,
    bottom,
    moveToFraction,
    retry,
  }
}
