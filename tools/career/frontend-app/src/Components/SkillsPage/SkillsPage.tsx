import { useEffect, useState } from 'react'
import { fetchPersonaDetails, fetchPersonas, fetchProfiles, type Persona, type Profile } from '../../api'
import { SparkIcon } from '../../Shared/Icons/icons'
import { PageHeaderCard } from '../../Shared/PageHeader/PageHeaderCard'
import { PageTips } from '../../Shared/PageTips/PageTips'
import { PERSONAS_PATH, skillsPathFor } from '../../paths'
import { plural } from '../CompaniesPage/companyStats'
import { accentFor } from '../JobsPage/jobsTheme'
import { PersonaList } from '../Persona/PersonaList'
import { previewItems } from '../Persona/personaListRules'

// How many of a persona's skills the list shows before "+N more".
const PREVIEW_SKILLS = 6

// The Skills page: the personas as a list (grouped by profile), each with how many skills it has and a few of
// them; a click opens that persona's own skills page (/tools/career/skills/:personaId), where they are edited.
// Nothing is edited here, and there is no dropdown. See plan/ai/tools/career/step-119-skills-per-persona-pages.md.
export function SkillsPage() {
  const [personas, setPersonas] = useState<Persona[]>([])
  const [profiles, setProfiles] = useState<Profile[]>([])
  // Each persona's skills by persona id; a persona whose skills could not be read is missing.
  const [skills, setSkills] = useState<Record<string, string[] | undefined>>({})
  const [error, setError] = useState('')
  const [loaded, setLoaded] = useState(false)

  useEffect(() => {
    Promise.all([fetchPersonas(), fetchProfiles()])
      .then(([personaList, profileList]) => {
        setPersonas(personaList)
        setProfiles(profileList)
        void Promise.all(personaList.map((p) => fetchPersonaDetails(p.id).catch(() => null))).then((all) => {
          const next: Record<string, string[] | undefined> = {}
          personaList.forEach((p, i) => {
            const details = all[i]
            if (details) next[p.id] = details.skills
          })
          setSkills(next)
        })
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
      .finally(() => {
        setLoaded(true)
      })
  }, [])

  const total = Object.values(skills).reduce((sum, list) => sum + (list?.length ?? 0), 0)

  return (
    <div className="career-v-tilt career-a-glow career-t-wave max-w-7xl p-6 font-sans text-gray-900">
      <PageHeaderCard
        icon={<SparkIcon />}
        title="Skills"
        description="Pick a persona to see and edit its skills. The AI reads them when matching you against job postings."
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
              , then add its skills here.
            </p>
          )}
          {personas.length > 0 && (
            <>
              <p className="mb-4 text-xs text-gray-500">{plural(total, 'skill', 'skills')} across these personas.</p>
              <PersonaList
                personas={personas}
                profiles={profiles}
                hrefFor={(p) => skillsPathFor(p.id)}
                renderExtra={(p) => {
                  const list = skills[p.id]
                  if (!list) return null
                  const { shown, more } = previewItems(list, PREVIEW_SKILLS)
                  return (
                    <span className="mt-1.5 flex flex-wrap items-center gap-1.5 text-xs text-gray-500">
                      <span className="font-medium text-gray-700">{plural(list.length, 'skill', 'skills')}</span>
                      {shown.map((s) => {
                        const accent = accentFor(s)
                        return (
                          <span
                            key={s}
                            className="rounded-full px-2 py-0.5 text-[11px] font-medium"
                            style={{ background: accent.soft, color: accent.text }}
                          >
                            {s}
                          </span>
                        )
                      })}
                      {more > 0 && <span>+{more} more</span>}
                    </span>
                  )
                }}
              />
            </>
          )}
        </div>
        <aside className="order-2 w-full shrink-0 lg:order-1 lg:sticky lg:top-6 lg:w-72">
          <PageTips page="skills" orientation="vertical" />
        </aside>
      </div>
    </div>
  )
}
