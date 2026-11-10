import { useState } from 'react'
import { TimelineIcon } from '../../Shared/Icons/icons'
import { PageHeaderCard } from '../../Shared/PageHeader/PageHeaderCard'
import { PageTips } from '../../Shared/PageTips/PageTips'
import { EXPERIENCE_PATH, PERSONAS_PATH, experiencePathFor } from '../../paths'
import { ProfileBar } from '../ProfileLibrary/ProfileBar'
import { experienceCountText } from '../ProfileLibrary/libraryRules'
import { useProfileChoice } from '../ProfileLibrary/useProfileChoice'
import { BackToProfile } from '../ProfileItems/BackToProfile'
import { ProfileExperienceEditor } from './ProfileExperienceEditor'

// The Experience page: the profile's own library of work experience (plan 173). It is global to the profile; each persona uses some of
// it (and may word an entry its own way), and may also have entries of its own, on the persona's own page
// (/tools/career/experience/:personaId). The profile comes from the address (?profileId=), with a dropdown when there is more than one.
export function ExperiencePage() {
  const { profiles, personas, profileId, select, error, loaded } = useProfileChoice(EXPERIENCE_PATH)
  const [count, setCount] = useState<number | null>(null)

  return (
    <div className="career-v-fade career-a-sheen career-t-type max-w-7xl p-6 font-sans text-gray-900">
      {profileId !== '' && <BackToProfile profileId={profileId} />}
      <PageHeaderCard
        icon={<TimelineIcon />}
        title="Experience"
        description="The work history of your profile, shared by all its personas. A persona uses some of it, and can have entries of its own. The AI reads it when matching you against job postings."
        pill={loaded && error === '' && profileId !== '' && count !== null ? { text: experienceCountText(count) } : undefined}
      />
      <div className="flex flex-col gap-5 lg:flex-row lg:items-start">
        <div className="order-1 min-w-0 flex-1 lg:order-2">
          {error !== '' && <p className="mb-3 text-sm text-red-700">{error}</p>}
          {loaded && error === '' && profiles.length === 0 && (
            <p className="rounded-3xl border border-gray-100 bg-gray-50 p-8 text-center text-sm text-gray-500">
              There is no profile yet. Create one on the Profiles page first.
            </p>
          )}
          {profileId !== '' && (
            <>
              <ProfileBar
                profiles={profiles}
                personas={personas}
                profileId={profileId}
                onSelect={select}
                personaHref={experiencePathFor}
                personaLabel="Experience of a persona:"
              />
              <ProfileExperienceEditor profileId={profileId} onCount={setCount} />
              {personas.length === 0 && (
                <p className="mt-3 text-xs text-gray-500">
                  A persona picks its entries from here.{' '}
                  <a
                    href={PERSONAS_PATH}
                    onClick={(e) => {
                      e.preventDefault()
                      window.__cinqoToolBridge.navigate(PERSONAS_PATH)
                    }}
                    className="font-medium text-blue-700 hover:underline"
                  >
                    Create a persona
                  </a>
                  .
                </p>
              )}
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
