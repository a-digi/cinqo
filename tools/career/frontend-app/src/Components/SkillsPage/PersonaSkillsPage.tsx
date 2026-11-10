import { useEffect, useState } from 'react'
import { fetchPersonas, fetchProfiles, type Persona, type Profile } from '../../api'
import { SparkIcon } from '../../Shared/Icons/icons'
import { PageHeaderCard } from '../../Shared/PageHeader/PageHeaderCard'
import { PageTips } from '../../Shared/PageTips/PageTips'
import { SKILLS_PATH } from '../../paths'
import { plural } from '../CompaniesPage/companyStats'
import { buildProfileSearch } from '../ProfileLibrary/libraryRules'
import { profileLabel } from '../ProfileSwitcher/profileLabel'
import { PersonaSkillsEditor } from './PersonaSkillsEditor'

// One persona's skills on their own page and address (/tools/career/skills/:personaId): the persona comes from
// the address, not from a dropdown, so the page can be linked to, bookmarked and reloaded. The editing is
// PersonaSkillsEditor's. An id that is not a persona shows "Persona not found". See
// plan/ai/tools/career/step-119-skills-per-persona-pages.md.
export function PersonaSkillsPage({ personaId }: { personaId: string }) {
  const [persona, setPersona] = useState<Persona | null>(null)
  const [profile, setProfile] = useState<Profile | null>(null)
  const [error, setError] = useState('')
  const [loaded, setLoaded] = useState(false)
  const [count, setCount] = useState<number | null>(null)

  useEffect(() => {
    setPersona(null)
    setProfile(null)
    setError('')
    setLoaded(false)
    fetchPersonas()
      .then((list) => {
        const found = list.find((p) => p.id === personaId)
        if (!found) {
          setError('Target role not found')
          return
        }
        setPersona(found)
        fetchProfiles()
          .then((profiles) => {
            setProfile(profiles.find((p) => p.id === found.profileId) ?? null)
          })
          .catch(() => {
            setProfile(null)
          })
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
      .finally(() => {
        setLoaded(true)
      })
  }, [personaId])

  const backPath = `${SKILLS_PATH}${buildProfileSearch(persona?.profileId ?? '')}`
  const backLink = (
    <a
      href={backPath}
      onClick={(e) => {
        e.preventDefault()
        window.__cinqoToolBridge.navigate(backPath)
      }}
      className="mb-3 inline-block text-sm font-medium text-blue-700 hover:underline"
    >
      ← Skills of the profile
    </a>
  )

  if (!persona) {
    return (
      <div className="max-w-3xl p-6 font-sans text-gray-900">
        {backLink}
        <div className="text-sm text-gray-500">{loaded ? error : 'Loading…'}</div>
      </div>
    )
  }

  return (
    <div className="career-v-tilt career-a-glow career-t-wave max-w-7xl p-6 font-sans text-gray-900">
      {backLink}
      <PageHeaderCard
        icon={<SparkIcon />}
        title={`Skills: ${persona.name}`}
        description={`${profile ? `${profileLabel(profile)} · ` : ''}The skills this target role uses from the profile’s library, and the ones only it has. The AI reads them when matching you against job postings.`}
        pill={count !== null ? { text: plural(count, 'skill', 'skills') } : undefined}
      />
      {/* The tips sit in a column of their own beside the page (under it on a narrow window). */}
      <div className="flex flex-col gap-5 lg:flex-row lg:items-start">
        <div className="order-1 min-w-0 flex-1 lg:order-2">
          <PersonaSkillsEditor personaId={persona.id} profileId={persona.profileId} onCount={setCount} />
        </div>
        <aside className="order-2 w-full shrink-0 lg:order-1 lg:sticky lg:top-6 lg:w-72">
          <PageTips page="skills" orientation="vertical" />
        </aside>
      </div>
    </div>
  )
}
