import { useEffect, useState } from 'react'
import { fetchPersonas, fetchProfiles, type Persona, type Profile } from '../../api'
import { PERSONAS_PATH } from '../../paths'
import { PersonaList } from '../Persona/PersonaList'

// Who a new CV is for: the personas as a list (grouped by profile, the chosen one marked), instead of a
// dropdown. A pick is reported through onSelect; what a pick does (stay on the page, or open that persona's own
// builder address) is the builder's decision. See plan/ai/tools/career/step-123-cv-builder-persona-list.md.
export function PersonaPicker({ personaId, onSelect }: { personaId: string | null; onSelect: (persona: Persona) => void }) {
  const [personas, setPersonas] = useState<Persona[]>([])
  const [profiles, setProfiles] = useState<Profile[]>([])
  const [error, setError] = useState('')
  const [loaded, setLoaded] = useState(false)

  useEffect(() => {
    Promise.all([fetchPersonas(), fetchProfiles()])
      .then(([personaList, profileList]) => {
        setPersonas(personaList)
        setProfiles(profileList)
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
      .finally(() => {
        setLoaded(true)
      })
  }, [])

  return (
    <section aria-label="Persona">
      <h3 className="mb-2 text-xs font-semibold uppercase tracking-wide text-gray-500">Who is this CV for?</h3>
      {error !== '' && <p className="mb-2 text-sm text-red-700">{error}</p>}
      {loaded && error === '' && personas.length === 0 && (
        <p className="text-sm text-gray-500">
          There are no personas yet.{' '}
          <a
            href={PERSONAS_PATH}
            onClick={(e) => {
              e.preventDefault()
              window.__cinqoToolBridge.navigate(PERSONAS_PATH)
            }}
            className="font-medium text-blue-700 hover:underline"
          >
            Create one on the Personas page
          </a>{' '}
          first.
        </p>
      )}
      {personas.length > 0 && <PersonaList personas={personas} profiles={profiles} selectedId={personaId} onSelect={onSelect} />}
    </section>
  )
}
