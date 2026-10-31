import { useEffect, useState } from 'react'
import { fetchExperience, fetchPersonas, fetchProfiles, type CareerExperience, type Persona, type Profile } from '../../api'
import { TimelineIcon } from '../../Shared/Icons/icons'
import { PageHeaderCard } from '../../Shared/PageHeader/PageHeaderCard'
import { PageTips } from '../../Shared/PageTips/PageTips'
import { PERSONAS_PATH, experiencePathFor } from '../../paths'
import { plural } from '../CompaniesPage/companyStats'
import { PersonaList } from '../Persona/PersonaList'
import { previewItems } from '../Persona/personaListRules'

// How many of a persona's roles the list shows before "+N more".
const PREVIEW_ROLES = 3

// The Experience page: the personas as a list (grouped by profile), each with how many entries it has and its
// first few roles; a click opens that persona's own experience page (/tools/career/experience/:personaId), where
// they are edited. Nothing is edited here, and there is no dropdown. See
// plan/ai/tools/career/step-120-experience-per-persona-pages.md.
export function ExperiencePage() {
  const [personas, setPersonas] = useState<Persona[]>([])
  const [profiles, setProfiles] = useState<Profile[]>([])
  // Each persona's entries by persona id; a persona whose entries could not be read is missing.
  const [experience, setExperience] = useState<Record<string, CareerExperience[] | undefined>>({})
  const [error, setError] = useState('')
  const [loaded, setLoaded] = useState(false)

  useEffect(() => {
    Promise.all([fetchPersonas(), fetchProfiles()])
      .then(([personaList, profileList]) => {
        setPersonas(personaList)
        setProfiles(profileList)
        void Promise.all(personaList.map((p) => fetchExperience(p.id).catch(() => null))).then((all) => {
          const next: Record<string, CareerExperience[] | undefined> = {}
          personaList.forEach((p, i) => {
            const entries = all[i]
            if (entries) next[p.id] = entries
          })
          setExperience(next)
        })
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
      .finally(() => {
        setLoaded(true)
      })
  }, [])

  const total = Object.values(experience).reduce((sum, list) => sum + (list?.length ?? 0), 0)

  return (
    <div className="career-v-fade career-a-sheen career-t-type max-w-7xl p-6 font-sans text-gray-900">
      <PageHeaderCard
        icon={<TimelineIcon />}
        title="Experience"
        description="Pick a persona to see and edit its work history. The AI reads it when matching you against job postings."
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
              , then add its experience here.
            </p>
          )}
          {personas.length > 0 && (
            <>
              <p className="mb-4 text-xs text-gray-500">{plural(total, 'entry', 'entries')} across these personas.</p>
              <PersonaList
                personas={personas}
                profiles={profiles}
                hrefFor={(p) => experiencePathFor(p.id)}
                renderExtra={(p) => {
                  const list = experience[p.id]
                  if (!list) return null
                  const roles = list.map((e) => `${e.title} at ${e.company}`)
                  const { shown, more } = previewItems(roles, PREVIEW_ROLES)
                  return (
                    <span className="mt-1.5 block text-xs text-gray-500">
                      <span className="font-medium text-gray-700">{plural(list.length, 'entry', 'entries')}</span>
                      {shown.length > 0 && <span> · {shown.join(' · ')}</span>}
                      {more > 0 && <span> · +{more} more</span>}
                    </span>
                  )
                }}
              />
            </>
          )}
        </div>
        <aside className="order-2 w-full shrink-0 lg:order-1 lg:sticky lg:top-6 lg:w-72">
          <PageTips page="experience" orientation="vertical" />
        </aside>
      </div>
    </div>
  )
}
