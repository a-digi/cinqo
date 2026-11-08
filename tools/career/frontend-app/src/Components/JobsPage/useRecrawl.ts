import { useCallback, useEffect, useRef, useState } from 'react'
import { cancelRecrawl, fetchRecrawlStatus, RecrawlRefused, requestRecrawl } from '../../api'
import { batchStorageKey, isRunning, parseStoredBatch, shouldReload, skippedText, type BatchStatus, type SkippedJob } from './recrawlRules'

// How often a running batch is asked for its progress, and how many of its jobs the details list asks for (the numbers need none).
const POLL_MS = 2000
const DETAILS_LIMIT = 500

export interface RecrawlControl {
  // The batch being followed, or null when there is none.
  batchId: string | null
  // Its progress: null until the first answer.
  status: BatchStatus | null
  // The jobs the request refused at once (they have no row in the batch).
  startSkipped: SkippedJob[]
  // A start that was refused, or a batch the server no longer knows; empty when all is well.
  error: string
  starting: boolean
  // True from the click on Stop until the crawls that were already running have finished (the waiting ones are cancelled at once).
  stopping: boolean
  // True while the batch has anything waiting or being crawled (or is just being started).
  running: boolean
  start: (jobIds: string[]) => Promise<void>
  stop: () => Promise<void>
  dismiss: () => void
  // The panel asks for the jobs' details only while its list is open (they are heavier than the numbers); closing the panel closes it.
  detailsOpen: boolean
  setDetailsOpen: (open: boolean) => void
}

function readStored(scope: string): string | null {
  try {
    return parseStoredBatch(localStorage.getItem(batchStorageKey(scope)))
  } catch {
    return null
  }
}

function writeStored(scope: string, id: string | null) {
  try {
    if (id === null) localStorage.removeItem(batchStorageKey(scope))
    else localStorage.setItem(batchStorageKey(scope), id)
  } catch {
    // The page works without storage; the batch is just not remembered after a reload.
  }
}

// Starts, follows and stops a recrawl batch (step 148). The work runs on the server, so closing the tab changes nothing: the batch id
// is kept in the browser's storage and picked up again when the page opens. onJobFinished is called whenever more jobs have finished
// than at the last look (the list reloads so new descriptions show); onFinished once when a batch that was running is over.
export function useRecrawl(onJobFinished: () => void, onFinished: () => void, scope = ''): RecrawlControl {
  const [batchId, setBatchId] = useState<string | null>(() => readStored(scope))
  const [status, setStatus] = useState<BatchStatus | null>(null)
  const [startSkipped, setStartSkipped] = useState<SkippedJob[]>([])
  const [error, setError] = useState('')
  const [starting, setStarting] = useState(false)
  const [stopping, setStopping] = useState(false)
  const [detailsOpen, setDetailsOpenState] = useState(false)

  const callbacks = useRef({ onJobFinished, onFinished })
  callbacks.current = { onJobFinished, onFinished }
  const previousDone = useRef(0)
  const wasRunning = useRef(false)

  const forget = useCallback(() => {
    writeStored(scope, null)
    setBatchId(null)
    setStatus(null)
    setStartSkipped([])
    setStopping(false)
  }, [scope])

  useEffect(() => {
    if (batchId === null) return
    let stopped = false // set by the cleanup below, while a request may be in flight
    const isStopped = () => stopped
    let timer: ReturnType<typeof setTimeout> | undefined

    const tick = async () => {
      try {
        const s = await fetchRecrawlStatus(batchId, detailsOpen ? DETAILS_LIMIT : 1)
        if (isStopped()) return
        if (s === null) {
          setError('The server no longer knows this recrawl (the tool was restarted). The jobs it finished are already updated.')
          forget()
          return
        }
        const before = previousDone.current
        previousDone.current = s.done
        setStatus(s)
        if (!isRunning(s)) setStopping(false)
        if (shouldReload(before, s)) callbacks.current.onJobFinished()
        if (isRunning(s)) {
          wasRunning.current = true
          timer = setTimeout(() => void tick(), POLL_MS)
          return
        }
        // Over: ask once for the jobs' details so the result can list them, then tell the page.
        if (!detailsOpen) {
          const full = await fetchRecrawlStatus(batchId, DETAILS_LIMIT)
          if (isStopped()) return
          if (full !== null) setStatus(full)
        }
        if (wasRunning.current) {
          wasRunning.current = false
          callbacks.current.onFinished()
        }
      } catch {
        if (!isStopped()) timer = setTimeout(() => void tick(), POLL_MS * 2)
      }
    }
    void tick()
    return () => {
      stopped = true
      if (timer !== undefined) clearTimeout(timer)
    }
  }, [batchId, detailsOpen, forget])

  const start = useCallback(
    async (jobIds: string[]) => {
      setError('')
      setStopping(false)
      setStarting(true)
      try {
        const started = await requestRecrawl(jobIds)
        previousDone.current = 0
        wasRunning.current = true
        writeStored(scope, started.batchId)
        setStartSkipped(started.skipped)
        // Something to show at once, before the first poll answers.
        setStatus({
          total: started.accepted,
          queued: started.accepted,
          running: 0,
          done: 0,
          cancelled: 0,
          counts: { refreshed: 0, unchanged: 0, filled: 0, kept: 0, failed: 0, skipped: 0 },
          jobs: [],
        })
        setBatchId(started.batchId)
      } catch (err) {
        if (err instanceof RecrawlRefused) setError(`${err.message}. ${skippedText(err.skipped)}.`)
        else setError(err instanceof Error ? err.message : String(err))
      } finally {
        setStarting(false)
      }
    },
    [scope],
  )

  const stop = useCallback(async () => {
    if (batchId === null) return
    setStopping(true)
    try {
      await cancelRecrawl(batchId)
    } catch (err) {
      setStopping(false)
      setError(err instanceof Error ? err.message : String(err))
    }
  }, [batchId])

  const dismiss = useCallback(() => {
    setError('')
    setDetailsOpenState(false)
    forget()
  }, [forget])

  return {
    batchId,
    status,
    startSkipped,
    error,
    starting,
    // Also true when the batch is winding down without this tab having clicked Stop (another tab, a reload): nothing waits, some are still crawled.
    stopping: stopping || (status !== null && status.queued === 0 && status.running > 0 && status.cancelled > 0),
    running: starting || (status !== null && isRunning(status)),
    start,
    stop,
    dismiss,
    detailsOpen,
    setDetailsOpen: setDetailsOpenState,
  }
}
