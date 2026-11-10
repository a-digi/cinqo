import { useEffect, useState } from 'react'
import { fetchPersonaDetails, type PersonaDetailsResult } from '../../../api'
import { TargetIcon } from '../../../Shared/Icons/icons'
import { PageHeaderCard } from '../../../Shared/PageHeader/PageHeaderCard'
import { PageTips } from '../../../Shared/PageTips/PageTips'
import { PERSONAS_PATH, PERSONA_DETAILS_PATH, personaDetailsPathFor } from '../../../paths'
import { plural } from '../../CompaniesPage/companyStats'
import { PersonaList } from '../PersonaList'
import { ProfileBar } from '../../ProfileLibrary/ProfileBar'
import { personasOfProfile } from '../../ProfileLibrary/libraryRules'
import { useProfileChoice } from '../../ProfileLibrary/useProfileChoice'
import { BackToProfile } from '../../ProfileItems/BackToProfile'
import { positioningCompleteness } from './detailsStats'
import { ProfileSummaryCard } from './ProfileSummaryCard'
import { isOwn, markText } from '../../GoalsPage/goalsRules'

// The Summary page (plan 174): the summary of the chosen profile, edited once at the top, and below it the profile's personas as a list,
// each with its headline and whether its summary follows the profile's or is its own wording; a click opens that persona's own page
// (/tools/career/persona-details/:personaId), where headline, location and the summary are edited. The goals have their own page.
export function PersonaDetailsListPage() {
  const { profiles, personas: allPersonas, profileId, select, error: loadError, loaded } = useProfileChoice(PERSONA_DETAILS_PATH)
  const personas = personasOfProfile(allPersonas, profileId)
  // Each persona's details by persona id; a persona whose details could not be read is missing.
  const [details, setDetails] = useState<Record<string, PersonaDetailsResult | undefined>>({})
  // Bumped after the profile's summary is saved: the personas are read again (the ones that follow show the new text).
  const [version, setVersion] = useState(0)
  const personaKey = personas.map((p) => p.id).join(',')

  useEffect(() => {
    const ids = personaKey === '' ? [] : personaKey.split(',')
    void Promise.all(ids.map((id) => fetchPersonaDetails(id).catch(() => null))).then((all) => {
      const next: Record<string, PersonaDetailsResult | undefined> = {}
      ids.forEach((id, i) => {
        const result = all[i]
        if (result) next[id] = result
      })
      setDetails(next)
    })
  }, [personaKey, version])
  const error = loadError

  return (
    <div className="career-v-wipe career-a-float career-t-underline max-w-7xl p-6 font-sans text-gray-900">
      {profileId !== '' && <BackToProfile profileId={profileId} />}
      <PageHeaderCard
        icon={<TargetIcon />}
        title="Summary"
        description="The summary of the profile, and how each of its personas presents itself. A persona uses the profile’s summary unless it has its own wording. The AI reads it when matching jobs and writing CVs."
        pill={loaded && error === '' && profileId !== '' ? { text: plural(personas.length, 'persona', 'personas') } : undefined}
      />
      {/* The tips sit in a column of their own beside the list (under it on a narrow window). */}
      <div className="flex flex-col gap-5 lg:flex-row lg:items-start">
        <div className="order-1 min-w-0 flex-1 lg:order-2">
          {error !== '' && <p className="mb-3 text-sm text-red-700">{error}</p>}
          {profileId !== '' && (
            <>
              <ProfileBar
                profiles={profiles}
                personas={[]}
                profileId={profileId}
                onSelect={select}
                personaHref={() => PERSONA_DETAILS_PATH}
                personaLabel=""
              />
              <ProfileSummaryCard
                profileId={profileId}
                onSaved={() => {
                  setVersion((v) => v + 1)
                }}
              />
            </>
          )}
          {loaded && error === '' && profiles.length === 0 && (
            <p className="rounded-3xl border border-gray-100 bg-gray-50 p-8 text-center text-sm text-gray-500">
              There is no profile yet. Create one on the Profiles page first.
            </p>
          )}
          {loaded && error === '' && profileId !== '' && personas.length === 0 && (
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
              , then word its summary here.
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
                const c = positioningCompleteness({
                  headline: saved?.headline ?? '',
                  summary: saved?.summary ?? '',
                  location: saved?.location ?? '',
                })
                const headline = saved?.headline.trim() ?? ''
                return (
                  <span className="mt-1.5 block text-xs text-gray-500">
                    <span className="font-medium text-gray-700">
                      {c.filled} of {c.total} fields filled
                    </span>
                    {headline !== '' && <span> · {headline}</span>}
                    <span> · {markText(isOwn(saved?.overridden, 'summary'))}</span>
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
