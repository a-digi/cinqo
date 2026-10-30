import { useCallback, useEffect, useRef, useState } from 'react'
import { ApiError } from '../../../api/client'
import { fetchFileSearch, type SearchMatch, type SearchResult } from '../../../api/fileReader'
import { describeFailure, startMatch, stepMatch } from './terminalRules'

const WAIT_MS = 400
const MAX_WAITS = 150
// Each empty page covers up to 64 MiB, so this reaches far beyond the largest file the reader accepts (1 GiB).
const MAX_EMPTY_PAGES = 64

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

export interface FileSearchState {
  // The text that was searched for (what the matches belong to); '' when no search has run.
  query: string
  caseSensitive: boolean
  matches: SearchMatch[]
  // The match the viewer is on, -1 when none.
  index: number
  // False while the file has more matches the server has not been asked for yet.
  done: boolean
  searching: boolean
  error: string | null
  run: (text: string, caseSensitive: boolean, anchorLine: number) => void
  next: () => void
  prev: () => void
  clear: () => void
}

// Searching a file by pages: the server answers with up to 50 matches and where to continue, and "next"
// asks for the following page only when the person steps past what is loaded. A search always starts at the
// top of the file and begins on the first match at or after the line being looked at. A new search, or a
// different file, drops the old one and cancels anything in flight.
export function useFileSearch(source: string, fileRef: string): FileSearchState {
  const [query, setQuery] = useState('')
  const [caseSensitive, setCaseSensitive] = useState(false)
  const [matches, setMatches] = useState<SearchMatch[]>([])
  const [index, setIndex] = useState(-1)
  const [done, setDone] = useState(false)
  const [searching, setSearching] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const nextFrom = useRef(1)
  const ctl = useRef<AbortController | null>(null)

  const cancel = useCallback(() => {
    ctl.current?.abort()
    ctl.current = null
  }, [])

  const clear = useCallback(() => {
    cancel()
    setQuery('')
    setMatches([])
    setIndex(-1)
    setDone(false)
    setSearching(false)
    setError(null)
    nextFrom.current = 1
  }, [cancel])

  // A different file: nothing of the old search applies.
  useEffect(() => {
    clear()
    return cancel
  }, [source, fileRef, clear, cancel])

  // One page of results from `from`, waiting for the index when the server says it has not got that far.
  const fetchPage = useCallback(
    async (text: string, sensitive: boolean, from: number, signal: AbortSignal) => {
      const aborted = () => signal.aborted
      for (let waits = 0; ; waits++) {
        try {
          return await fetchFileSearch(source, fileRef, text, from, sensitive, signal)
        } catch (err) {
          if (err instanceof ApiError && err.status === 409 && waits < MAX_WAITS && !aborted()) {
            await sleep(WAIT_MS, signal)
            if (aborted()) return null
            continue
          }
          throw err
        }
      }
    },
    [source, fileRef],
  )

  // A page can come back empty yet not done: the server stopped after its byte or time budget without finding
  // anything. That is not "no matches" (the rest of a big file has not been looked at), so keep going until a
  // match turns up, the file ends, or the search is cancelled.
  const fetchUntilMatch = useCallback(
    async (text: string, sensitive: boolean, startFrom: number, signal: AbortSignal) => {
      let from = startFrom
      let last: SearchResult | null = null
      for (let pages = 0; pages < MAX_EMPTY_PAGES; pages++) {
        const page = await fetchPage(text, sensitive, from, signal)
        if (!page) return null
        last = page
        if (page.matches.length > 0 || page.done || page.nextFrom <= from) break
        from = page.nextFrom
      }
      return last
    },
    [fetchPage],
  )

  const run = useCallback(
    (text: string, sensitive: boolean, anchorLine: number) => {
      cancel()
      const mine = new AbortController()
      // a function, so the checks after each await are not narrowed to "never aborted"
      const cancelled = () => mine.signal.aborted
      ctl.current = mine
      setQuery(text)
      setCaseSensitive(sensitive)
      setMatches([])
      setIndex(-1)
      setDone(false)
      setError(null)
      setSearching(true)
      void (async () => {
        try {
          const page = await fetchUntilMatch(text, sensitive, 1, mine.signal)
          if (!page || cancelled()) return
          nextFrom.current = page.nextFrom
          setMatches(page.matches)
          setDone(page.done)
          setIndex(
            startMatch(
              page.matches.map((m) => m.n),
              anchorLine,
            ),
          )
          setSearching(false)
        } catch (err) {
          if (cancelled()) return
          setError(describeFailure(err instanceof ApiError ? err.status : undefined))
          setSearching(false)
        }
      })()
    },
    [cancel, fetchUntilMatch],
  )

  const next = useCallback(() => {
    if (index + 1 < matches.length) {
      setIndex(stepMatch(index, matches.length, 1))
      return
    }
    if (done || searching || query === '') return
    // past what is loaded: ask for the next page, then step onto its first match
    const mine = new AbortController()
    const cancelled = () => mine.signal.aborted
    ctl.current = mine
    setSearching(true)
    void (async () => {
      try {
        const page = await fetchUntilMatch(query, caseSensitive, nextFrom.current, mine.signal)
        if (!page || cancelled()) return
        nextFrom.current = page.nextFrom
        setMatches((prev) => [...prev, ...page.matches])
        setDone(page.done)
        if (page.matches.length > 0) setIndex(index + 1)
        setSearching(false)
      } catch (err) {
        if (cancelled()) return
        setError(describeFailure(err instanceof ApiError ? err.status : undefined))
        setSearching(false)
      }
    })()
  }, [index, matches.length, done, searching, query, caseSensitive, fetchUntilMatch])

  const prev = useCallback(() => {
    if (matches.length > 0) setIndex(stepMatch(index, matches.length, -1))
  }, [index, matches.length])

  return { query, caseSensitive, matches, index, done, searching, error, run, next, prev, clear }
}
