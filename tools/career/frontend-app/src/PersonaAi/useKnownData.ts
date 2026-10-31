import { useEffect, useState } from 'react'
import { fetchPersonaDetails } from '../api'
import { buildKnown, hasBasis, type KnownData } from './knownData'

export type KnownState = { status: 'loading' } | { status: 'failed' } | { status: 'ready'; known: KnownData; comparable: boolean }

// The skills, employers and titles of the given existing personas, read through the existing
// persona details route (step 113). One failed read makes the whole comparison unavailable: a
// partial list would mark things that are in the user's data.
export function useKnownData(personaIds: string[]): KnownState {
  const [state, setState] = useState<KnownState>({ status: 'loading' })
  const key = personaIds.join(',')

  useEffect(() => {
    let cancelled = false
    setState({ status: 'loading' })
    Promise.all(personaIds.map((id) => fetchPersonaDetails(id)))
      .then((all) => {
        if (cancelled) return
        const known = buildKnown(all)
        setState({ status: 'ready', known, comparable: hasBasis(known) })
      })
      .catch(() => {
        if (!cancelled) setState({ status: 'failed' })
      })
    return () => {
      cancelled = true
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [key])

  return state
}
