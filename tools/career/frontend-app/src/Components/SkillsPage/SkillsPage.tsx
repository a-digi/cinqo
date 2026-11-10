import { useState } from 'react'
import { SparkIcon } from '../../Shared/Icons/icons'
import { PageHeaderCard } from '../../Shared/PageHeader/PageHeaderCard'
import { PageTips } from '../../Shared/PageTips/PageTips'
import { PERSONAS_PATH, SKILLS_PATH, skillsPathFor } from '../../paths'
import { ProfileBar } from '../ProfileLibrary/ProfileBar'
import { skillCountText } from '../ProfileLibrary/libraryRules'
import { useProfileChoice } from '../ProfileLibrary/useProfileChoice'
import { BackToProfile } from '../ProfileItems/BackToProfile'
import { ProfileSkillsEditor } from './ProfileSkillsEditor'

// The Skills page: the profile's own library of skills (plan 173). They are global to the profile; each persona uses some of them,
// and may also have skills of its own, on the persona's own page (/tools/career/skills/:personaId). The profile comes from the
// address (?profileId=), with a dropdown when there is more than one.
export function SkillsPage() {
  const { profiles, personas, profileId, select, error, loaded } = useProfileChoice(SKILLS_PATH)
  const [count, setCount] = useState<number | null>(null)

  return (
    <div className="career-v-tilt career-a-glow career-t-wave max-w-7xl p-6 font-sans text-gray-900">
      {profileId !== '' && <BackToProfile profileId={profileId} profiles={profiles} />}
      <PageHeaderCard
        icon={<SparkIcon />}
        title="Skills"
        description="The skills of your profile, shared by all its target roles. A target role uses some of them, and can have skills of its own. The AI reads them when matching you against job postings."
        pill={loaded && error === '' && profileId !== '' && count !== null ? { text: skillCountText(count) } : undefined}
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
                personaHref={skillsPathFor}
                personaLabel="Skills of a target role:"
              />
              <ProfileSkillsEditor profileId={profileId} onCount={setCount} />
              {personas.length === 0 && (
                <p className="mt-3 text-xs text-gray-500">
                  A target role picks its skills from here.{' '}
                  <a
                    href={PERSONAS_PATH}
                    onClick={(e) => {
                      e.preventDefault()
                      window.__cinqoToolBridge.navigate(PERSONAS_PATH)
                    }}
                    className="font-medium text-blue-700 hover:underline"
                  >
                    Create a target role
                  </a>
                  .
                </p>
              )}
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
