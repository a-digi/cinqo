import { useCallback, useState } from 'react'

const KEY = 'career.cvAiDismissed'

function read(): Set<string> {
  try {
    const raw: unknown = JSON.parse(localStorage.getItem(KEY) ?? '[]')
    return new Set(Array.isArray(raw) ? raw.filter((x): x is string => typeof x === 'string') : [])
  } catch {
    return new Set()
  }
}

// The failed CV AI runs the user dismissed (a per-viewer convenience kept in this browser): the
// failure notice hides, the run and its chat stay in the history.
export function useDismissedCvRuns(): [ReadonlySet<string>, (runId: string) => void] {
  const [dismissed, setDismissed] = useState<ReadonlySet<string>>(read)
  const dismiss = useCallback((runId: string) => {
    setDismissed((current) => {
      const next = new Set(current).add(runId)
      try {
        localStorage.setItem(KEY, JSON.stringify([...next]))
      } catch {
        // Not stored: the notice is hidden for this visit only.
      }
      return next
    })
  }, [])
  return [dismissed, dismiss]
}
