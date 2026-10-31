import { useEffect, useState } from 'react'
import { fetchPersonaDetails, fetchPersonas, fetchProfiles, type Persona, type PersonaDetailsResult, type Profile } from '../../../api'
import { TargetIcon } from '../../../Shared/Icons/icons'
import { PageHeaderCard } from '../../../Shared/PageHeader/PageHeaderCard'
import { PageTips } from '../../../Shared/PageTips/PageTips'
import { PERSONAS_PATH, personaDetailsPathFor } from '../../../paths'
import { plural } from '../../CompaniesPage/companyStats'
import { PersonaList } from '../PersonaList'
import { detailsCompleteness } from './detailsStats'

// The Summary & Goals page: the personas as a list (grouped by profile), each with how many of the six fields are
// filled in and its headline; a click opens that persona's own page (/tools/career/persona-details/:personaId),
// where the details are edited. Nothing is edited here, and there is no dropdown. See
// plan/ai/tools/career/step-121-summary-and-goals-per-persona-pages.md.
export function PersonaDetailsListPage() {
  const [personas, setPersonas] = useState<Persona[]>([])
  const [profiles, setProfiles] = useState<Profile[]>([])
  // Each persona's details by persona id; a persona whose details could not be read is missing.
  const [details, setDetails] = useState<Record<string, PersonaDetailsResult | undefined>>({})
  const [error, setError] = useState('')
  const [loaded, setLoaded] = useState(false)

  useEffect(() => {
    Promise.all([fetchPersonas(), fetchProfiles()])
      .then(([personaList, profileList]) => {
        setPersonas(personaList)
        setProfiles(profileList)
        void Promise.all(personaList.map((p) => fetchPersonaDetails(p.id).catch(() => null))).then((all) => {
          const next: Record<string, PersonaDetailsResult | undefined> = {}
          personaList.forEach((p, i) => {
            const result = all[i]
            if (result) next[p.id] = result
          })
          setDetails(next)
        })
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
      .finally(() => {
        setLoaded(true)
      })
  }, [])

  return (
    <div className="career-v-wipe career-a-float career-t-underline max-w-7xl p-6 font-sans text-gray-900">
      <PageHeaderCard
        icon={<TargetIcon />}
        title="Summary & Goals"
        description="Pick a persona to see and edit its positioning and what it is looking for. The AI reads it when matching jobs and writing CVs."
        pill={loaded && error === '' ? { text: plural(personas.length, 'persona', 'personas') } : undefined}
      />
      {/* The tips sit in a column of their own beside the list (under it on a narrow window). */}
      <div className="flex flex-col gap-5 lg:flex-row lg:items-start">
        <div className="order-1 min-w-0 flex-1 lg:order-2">
          {error !== '' && <p className="mb-3 text-sm text-red-700">{error}</p>}
          {loaded && error === '' && personas.length === 0 && (
            <p className="rounded-3xl border border-gray-100 bg-gray-50 p-8 text-center text-sm text-gray-500">
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
              </a>
              , then write its summary and goals here.
            </p>
          )}
          {personas.length > 0 && (
            <PersonaList
              personas={personas}
              profiles={profiles}
              hrefFor={(p) => personaDetailsPathFor(p.id)}
              renderExtra={(p) => {
                const result = details[p.id]
                if (!result) return null
                const saved = result.personaDetails
                const c = detailsCompleteness({
                  headline: saved?.headline ?? '',
                  summary: saved?.summary ?? '',
                  location: saved?.location ?? '',
                  desiredTitles: saved?.desiredTitles ?? '',
                  desiredLocations: saved?.desiredLocations ?? '',
                  minSalary: saved?.minSalary ?? 0,
                })
                const headline = saved?.headline.trim() ?? ''
                return (
                  <span className="mt-1.5 block text-xs text-gray-500">
                    <span className="font-medium text-gray-700">
                      {c.filled} of {c.total} fields filled
                    </span>
                    {headline !== '' && <span> · {headline}</span>}
                  </span>
                )
              }}
            />
          )}
        </div>
        <aside className="order-2 w-full shrink-0 lg:order-1 lg:sticky lg:top-6 lg:w-72">
          <PageTips page="summary" orientation="vertical" />
        </aside>
      </div>
    </div>
  )
}
