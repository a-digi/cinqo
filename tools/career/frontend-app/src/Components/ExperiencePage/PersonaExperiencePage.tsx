import { useEffect, useState } from 'react'
import { fetchPersonas, fetchProfiles, type Persona, type Profile } from '../../api'
import { TimelineIcon } from '../../Shared/Icons/icons'
import { PageHeaderCard } from '../../Shared/PageHeader/PageHeaderCard'
import { PageTips } from '../../Shared/PageTips/PageTips'
import { EXPERIENCE_PATH } from '../../paths'
import { plural } from '../CompaniesPage/companyStats'
import { profileLabel } from '../ProfileSwitcher/profileLabel'
import { PersonaExperienceEditor } from './PersonaExperienceEditor'

// One persona's work history on its own page and address (/tools/career/experience/:personaId): the persona comes
// from the address, not from a dropdown, so the page can be linked to, bookmarked and reloaded. The editing is
// PersonaExperienceEditor's. An id that is not a persona shows "Persona not found". See
// plan/ai/tools/career/step-120-experience-per-persona-pages.md.
export function PersonaExperiencePage({ personaId }: { personaId: string }) {
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
          setError('Persona not found')
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

  const backLink = (
    <a
      href={EXPERIENCE_PATH}
      onClick={(e) => {
        e.preventDefault()
        window.__cinqoToolBridge.navigate(EXPERIENCE_PATH)
      }}
      className="mb-3 inline-block text-sm font-medium text-blue-700 hover:underline"
    >
      ← All experience
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
    <div className="career-v-fade career-a-sheen career-t-type max-w-7xl p-6 font-sans text-gray-900">
      {backLink}
      <PageHeaderCard
        icon={<TimelineIcon />}
        title={`Experience: ${persona.name}`}
        description={`${profile ? `${profileLabel(profile)} · ` : ''}The work history on this persona. The AI reads it when matching you against job postings.`}
        pill={count !== null ? { text: plural(count, 'entry', 'entries') } : undefined}
      />
      {/* The tips sit in a column of their own beside the page (under it on a narrow window). */}
      <div className="flex flex-col gap-5 lg:flex-row lg:items-start">
        <div className="order-1 min-w-0 flex-1 lg:order-2">
          <PersonaExperienceEditor personaId={persona.id} onCount={setCount} />
        </div>
        <aside className="order-2 w-full shrink-0 lg:order-1 lg:sticky lg:top-6 lg:w-72">
          <PageTips page="experience" orientation="vertical" />
        </aside>
      </div>
    </div>
  )
}
