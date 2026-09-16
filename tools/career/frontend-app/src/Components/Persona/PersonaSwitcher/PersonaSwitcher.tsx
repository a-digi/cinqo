import { useEffect, useState } from 'react'
import { fetchPersonas, fetchProfiles, type Persona } from '../../../api'
import { getPersonaIdFromLocalStorage, setPersonaIdInLocalStorage } from './repositoryLocalStorage'
import { profileLabel } from '../../ProfileSwitcher/profileLabel'
import { Dropdown } from '../../Dropdown/Dropdown'

const PERSONAS_PATH = '/tools/career/personas'

// Shared by PersonaDetailsPage/SkillsPage/ExperiencePage — resolves
// an "effective" persona id (the stored one if it's still valid, else
// the first persona in the list, else null if there are none at all)
// and reports it back to the parent page via onChange, which is what
// actually reloads that page's own details/skills/experience data.
//
// Left genuinely unfiltered by profile (step 11) — still lists every
// persona across every profile — but each option now reads "{persona
// name} — {profile name}" so personas from different job seekers are
// at least visually distinguishable in this one shared dropdown, a
// deliberate middle ground short of full profile-scoped filtering.
// See plan/ai/tools/career/step-09-persona-frontend.md and
// plan/ai/tools/career/step-11-job-seeker-profile-frontend.md.
export function PersonaSwitcher({ personaId, onChange }: { personaId: string | null; onChange: (id: string) => void }) {
  const [personas, setPersonas] = useState<Persona[] | null>(null)
  const [profileNames, setProfileNames] = useState<Record<string, string>>({})
  const [error, setError] = useState('')

  useEffect(() => {
    Promise.all([fetchPersonas(), fetchProfiles()])
      .then(([personaList, profileList]) => {
        setPersonas(personaList)
        setProfileNames(Object.fromEntries(profileList.map((p) => [p.id, profileLabel(p)])))
        if (personaList.length === 0) return
        const stored = getPersonaIdFromLocalStorage()
        const effective = personaList.find((p) => p.id === stored) ?? personaList[0]
        onChange(effective.id)
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  if (error) {
    return <div className="mb-4 text-sm text-red-700">{error}</div>
  }

  if (personas === null) {
    return null
  }

  if (personas.length === 0) {
    return (
      <div className="mb-4 rounded-md border border-gray-200 bg-gray-50 p-3 text-sm text-gray-700">
        No personas yet.{' '}
        <a href={PERSONAS_PATH} onClick={navigateToPersonas} className="font-medium underline">
          Create one to get started
        </a>
        .
      </div>
    )
  }

  return (
    <div className="mb-4 flex items-end gap-2 text-sm">
      <Dropdown
        label="Persona"
        options={personas.map((p) => ({
          value: p.id,
          label: profileNames[p.profileId] ? `${p.name} — ${profileNames[p.profileId]}` : p.name,
        }))}
        value={personaId}
        onChange={(id) => {
          setPersonaIdInLocalStorage(id)
          onChange(id)
        }}
      />
      <a href={PERSONAS_PATH} onClick={navigateToPersonas} className="pb-1 text-gray-500 underline hover:text-gray-700">
        Manage
      </a>
    </div>
  )
}

function navigateToPersonas(e: React.MouseEvent) {
  e.preventDefault()
  window.__cinqoToolBridge.navigate(PERSONAS_PATH)
}
