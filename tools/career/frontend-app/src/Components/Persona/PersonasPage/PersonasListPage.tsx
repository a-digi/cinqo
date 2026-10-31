import { useEffect, useState } from 'react'
import { fetchPersonas, fetchProfiles, type Persona, type Profile } from '../../../api'
import { LayersIcon } from '../../../Shared/Icons/icons'
import { PageHeaderCard } from '../../../Shared/PageHeader/PageHeaderCard'
import { PageTips } from '../../../Shared/PageTips/PageTips'
import { PROFILES_PATH, personasPathFor } from '../../../paths'
import { plural } from '../../CompaniesPage/companyStats'
import { ProfileList } from '../../ProfilesPage/ProfileList'
import { previewItems } from '../personaListRules'

// How many of a profile's persona names the list shows before "+N more".
const PREVIEW_PERSONAS = 4

// The Personas page: the profiles as a list, each with how many personas it has and their names; a click opens
// that profile's own personas page (/tools/career/personas/:profileId), where they are created, edited, reviewed
// and suggested by the AI. Nothing is edited here, and there is no dropdown. An older link of the shape
// ?profileId=…&suggestion=… (a chat message from before the profile had its own address) is sent on to the
// profile's page. See plan/ai/tools/career/step-124-personas-per-profile-pages.md.
export function PersonasListPage() {
  const [profiles, setProfiles] = useState<Profile[]>([])
  const [personas, setPersonas] = useState<Persona[]>([])
  const [error, setError] = useState('')
  const [loaded, setLoaded] = useState(false)

  useEffect(() => {
    const params = new URLSearchParams(window.location.search)
    const legacyProfile = params.get('profileId')
    if (legacyProfile) {
      const suggestion = params.get('suggestion')
      window.__cinqoToolBridge.navigate(
        `${personasPathFor(legacyProfile)}${suggestion ? `?suggestion=${encodeURIComponent(suggestion)}` : ''}`,
      )
      return
    }
    Promise.all([fetchProfiles(), fetchPersonas()])
      .then(([profileList, personaList]) => {
        setProfiles(profileList)
        setPersonas(personaList)
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
      .finally(() => {
        setLoaded(true)
      })
  }, [])

  return (
    <div className="career-v-flip career-a-sheen career-t-wave max-w-7xl p-6 font-sans text-gray-900">
      <PageHeaderCard
        icon={<LayersIcon />}
        title="Personas"
        description="Pick a profile to see its personas: one per role it is pursuing, each with its own summary, skills and experience."
        pill={loaded && error === '' ? { text: plural(profiles.length, 'profile', 'profiles') } : undefined}
      />
      {/* The tips sit in a column of their own beside the list (under it on a narrow window). */}
      <div className="flex flex-col gap-5 lg:flex-row lg:items-start">
        <div className="min-w-0 flex-1">
          {error !== '' && <p className="mb-3 text-sm text-red-700">{error}</p>}
          {loaded && error === '' && profiles.length === 0 && (
            <p className="rounded-3xl border border-gray-100 bg-gray-50 p-8 text-center text-sm text-gray-500">
              There are no profiles yet.{' '}
              <a
                href={PROFILES_PATH}
                onClick={(e) => {
                  e.preventDefault()
                  window.__cinqoToolBridge.navigate(PROFILES_PATH)
                }}
                className="font-medium text-blue-700 hover:underline"
              >
                Create one on the Profiles page
              </a>
              , then add its personas here.
            </p>
          )}
          {profiles.length > 0 && (
            <>
              <p className="mb-4 text-xs text-gray-500">{plural(personas.length, 'persona', 'personas')} across these profiles.</p>
              <ProfileList
                profiles={profiles}
                hrefFor={(p) => personasPathFor(p.id)}
                renderExtra={(p) => {
                  if (!loaded) return null
                  const names = personas.filter((x) => x.profileId === p.id).map((x) => x.name)
                  const { shown, more } = previewItems(names, PREVIEW_PERSONAS)
                  return (
                    <span className="mt-1.5 block text-xs text-gray-500">
                      <span className="font-medium text-gray-700">{plural(names.length, 'persona', 'personas')}</span>
                      {shown.length > 0 && <span> · {shown.join(' · ')}</span>}
                      {more > 0 && <span> · +{more} more</span>}
                    </span>
                  )
                }}
              />
            </>
          )}
        </div>
        <aside className="w-full shrink-0 lg:sticky lg:top-6 lg:w-72">
          <PageTips page="personas" orientation="vertical" />
        </aside>
      </div>
    </div>
  )
}
