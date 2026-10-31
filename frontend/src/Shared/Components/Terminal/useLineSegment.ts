import { useCallback, useEffect, useState } from 'react'
import { ApiError } from '../../../api/client'
import { fetchFileSegment, type FileSegment } from '../../../api/fileReader'
import { describeFailure } from './terminalRules'

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

export interface LineSegmentState {
  // The line being read in segments, or null when the viewer shows lines.
  line: number | null
  // The segment on screen (the one before it stays until the next arrives).
  segment: FileSegment | null
  loading: boolean
  error: string | null
  open: (line: number, offset: number) => void
  goTo: (offset: number) => void
  close: () => void
  retry: () => void
}

// Reads one long line a segment at a time by byte offset. A different file closes it. The segment on screen
// stays until its successor arrives, and a request for a part of the file the server has not indexed yet waits
// for it, as the window does.
export function useLineSegment(source: string, fileRef: string): LineSegmentState {
  const [line, setLine] = useState<number | null>(null)
  const [offset, setOffset] = useState(0)
  const [segment, setSegment] = useState<FileSegment | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [attempt, setAttempt] = useState(0)

  const close = useCallback(() => {
    setLine(null)
    setSegment(null)
    setError(null)
    setLoading(false)
  }, [])

  useEffect(() => {
    close()
  }, [source, fileRef, close])

  useEffect(() => {
    if (line === null) return
    const ctl = new AbortController()
    // a function, so the checks after each await are not narrowed to "never aborted"
    const cancelled = () => ctl.signal.aborted
    setLoading(true)
    setError(null)
    void (async () => {
      try {
        for (let waits = 0; ; waits++) {
          try {
            const next = await fetchFileSegment(source, fileRef, line, offset, ctl.signal)
            if (cancelled()) return
            setSegment(next)
            setLoading(false)
            return
          } catch (err) {
            if (err instanceof ApiError && err.status === 409 && waits < MAX_WAITS) {
              await sleep(WAIT_MS, ctl.signal)
              if (cancelled()) return
              continue
            }
            throw err
          }
        }
      } catch (err) {
        if (cancelled()) return
        setError(describeFailure(err instanceof ApiError ? err.status : undefined))
        setLoading(false)
      }
    })()
    return () => {
      ctl.abort()
    }
  }, [source, fileRef, line, offset, attempt])

  const open = useCallback((nextLine: number, nextOffset: number) => {
    setLine(nextLine)
    setOffset(Math.max(0, nextOffset))
  }, [])
  const goTo = useCallback((nextOffset: number) => {
    setOffset(Math.max(0, nextOffset))
  }, [])
  const retry = useCallback(() => {
    setAttempt((n) => n + 1)
  }, [])

  return { line, segment, loading, error, open, goTo, close, retry }
}
