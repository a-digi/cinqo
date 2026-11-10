import { useEffect, useState } from 'react'
import { fetchJobStatsRaw } from '../../api'
import { parseJobStats, type JobStats } from './jobStats'

export type JobStatsState = { state: 'loading'; stats: null } | { state: 'failed'; stats: null } | { state: 'ready'; stats: JobStats }

// The numbers of the Jobs page's statistics cards (GET /jobs/stats), fetched once for the whole page and refetched when `refreshKey` changes (a
// job was added, deleted, matched…). The four headline cards and the platform card both read it, so the page makes one request. While a later
// refetch fails the numbers already shown stay (plan 236).
export function useJobStats(refreshKey: string): JobStatsState {
  const [stats, setStats] = useState<JobStats | null>(null)
  const [state, setState] = useState<'loading' | 'ready' | 'failed'>('loading')

  useEffect(() => {
    let cancelled = false
    fetchJobStatsRaw()
      .then((raw) => {
        if (cancelled) return
        const parsed = parseJobStats(raw)
        setStats(parsed)
        setState(parsed ? 'ready' : 'failed')
      })
      .catch(() => {
        if (!cancelled) setState((prev) => (prev === 'ready' ? prev : 'failed'))
      })
    return () => {
      cancelled = true
    }
  }, [refreshKey])

  if (state === 'ready' && stats) return { state: 'ready', stats }
  return state === 'failed' ? { state: 'failed', stats: null } : { state: 'loading', stats: null }
}
