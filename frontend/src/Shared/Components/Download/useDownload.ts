import { useEffect, useState } from 'react'
import { ApiError } from '../../../api/client'
import { fetchDownload, type Download } from '../../../api/downloads'
import { isFinal } from './downloadText'

const ACTIVE_MS = 1000

export interface UseDownload {
  download: Download | null
  // Set when the download cannot be asked about any more (the job is forgotten, or the server cannot be reached).
  error: string
}

// Follows one download: asks for its progress every second while it is active and stops when it is over. With no id it does
// nothing. A job the server has forgotten (it only keeps finished jobs for a while) ends the polling with an error.
export function useDownload(id: string | null): UseDownload {
  const [state, setState] = useState<{ id: string | null; download: Download | null; error: string }>({
    id: null,
    download: null,
    error: '',
  })

  useEffect(() => {
    if (id === null) return
    const controller = new AbortController()
    let timer: number | undefined
    let cancelled = false

    const poll = () => {
      fetchDownload(id, controller.signal)
        .then((d) => {
          if (cancelled) return
          setState({ id, download: d, error: '' })
          if (!isFinal(d.state)) timer = window.setTimeout(poll, ACTIVE_MS)
        })
        .catch((err: unknown) => {
          if (cancelled || controller.signal.aborted) return
          const gone = err instanceof ApiError && err.status === 404
          setState((prev) => ({
            id,
            download: prev.id === id ? prev.download : null,
            error: gone ? 'This download is no longer listed.' : 'The progress could not be read.',
          }))
          if (!gone) timer = window.setTimeout(poll, ACTIVE_MS * 3)
        })
    }
    poll()

    return () => {
      cancelled = true
      controller.abort()
      if (timer !== undefined) window.clearTimeout(timer)
    }
  }, [id])

  if (id === null || state.id !== id) return { download: null, error: '' }
  return { download: state.download, error: state.error }
}
