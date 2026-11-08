import { useEffect, useState } from 'react'
import { fetchPersonaDetails, fetchProfileExperience, fetchProfileSkills } from '../api'
import { buildKnown, hasBasis, type KnownData } from './knownData'

export type KnownState = { status: 'loading' } | { status: 'failed' } | { status: 'ready'; known: KnownData; comparable: boolean }

// The skills, employers and titles of the given existing personas, read through the existing
// persona details route (step 113). One failed read makes the whole comparison unavailable: a
// partial list would mark things that are in the user's data.
export function useKnownData(personaIds: string[], profileId?: string): KnownState {
  const [state, setState] = useState<KnownState>({ status: 'loading' })
  const key = `${personaIds.join(',')}|${profileId ?? ''}`

  useEffect(() => {
    let cancelled = false
    setState({ status: 'loading' })
    // Plan 173: what the profile's library holds is known too, also the part no persona uses yet.
    const library =
      profileId === undefined
        ? Promise.resolve(null)
        : Promise.all([fetchProfileSkills(profileId), fetchProfileExperience(profileId)]).then(([skills, experience]) => ({
            skills: skills.map((s) => s.skill),
            experience,
          }))
    Promise.all([Promise.all(personaIds.map((id) => fetchPersonaDetails(id))), library])
      .then(([all, lib]) => {
        if (cancelled) return
        const known = buildKnown(lib === null ? all : [...all, lib])
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
