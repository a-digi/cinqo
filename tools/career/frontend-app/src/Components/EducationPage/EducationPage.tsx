import { useEffect, useState } from 'react'
import { fetchEducation, removeEducation, type Education } from '../../api'
import { EducationIcon, PlusIcon } from '../../Shared/Icons/icons'
import { PageTips } from '../../Shared/PageTips/PageTips'
import { PageHeaderCard } from '../../Shared/PageHeader/PageHeaderCard'
import { EDUCATION_ENTRY_PATH, EDUCATION_PATH } from '../../paths'
import { PAGE_ACCENT, accentFor, initialOf } from '../JobsPage/jobsTheme'
import { ProfileBar } from '../ProfileLibrary/ProfileBar'
import { useProfileChoice } from '../ProfileLibrary/useProfileChoice'
import { BackToProfile } from '../ProfileItems/BackToProfile'
import { degreeLine, educationCountText, isOngoing, periodText } from './educationRules'

// The Education page (plan 173): the education of the profile (schools, degrees, courses), global to it; personas do not choose it. The
// profile comes from the address (?profileId=), with a dropdown when there is more than one. Add and Edit open EducationEntryPage.
export function EducationPage() {
  const { profiles, profileId, select, error: loadError, loaded } = useProfileChoice(EDUCATION_PATH)
  const [entries, setEntries] = useState<Education[]>([])
  const [error, setError] = useState('')

  useEffect(() => {
    if (profileId === '') return
    setError('')
    fetchEducation(profileId)
      .then(setEntries)
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
  }, [profileId])

  function openEntry(id?: string) {
    const idPart = id ? `&id=${encodeURIComponent(id)}` : ''
    window.__cinqoToolBridge.navigate(`${EDUCATION_ENTRY_PATH}?profileId=${encodeURIComponent(profileId)}${idPart}`)
  }

  function handleRemove(id: string) {
    setError('')
    removeEducation(id)
      .then(() => {
        setEntries((prev) => prev.filter((e) => e.id !== id))
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
  }

  const shownError = loadError !== '' ? loadError : error

  return (
    <div className="career-v-zoom career-a-float career-t-type max-w-5xl p-6 font-sans text-gray-900">
      {profileId !== '' && <BackToProfile profileId={profileId} profiles={profiles} />}
      <PageHeaderCard
        icon={<EducationIcon />}
        title="Education"
        help={EDUCATION_PATH}
        description="The schools and degrees of your profile. They belong to the profile itself, so every target role and every CV can use them. Short trainings and certificates go on the Courses page."
        pill={loaded && shownError === '' && profileId !== '' ? { text: educationCountText(entries.length) } : undefined}
      />
      <div className="flex flex-col gap-5 lg:flex-row lg:items-start">
        <div className="order-1 min-w-0 flex-1 lg:order-2">
          {shownError !== '' && <p className="mb-3 text-sm text-red-700">{shownError}</p>}
          {loaded && loadError === '' && profiles.length === 0 && (
            <p className="rounded-3xl border border-gray-100 bg-gray-50 p-8 text-center text-sm text-gray-500">
              There is no profile yet. Create one on the Profiles page first.
            </p>
          )}
          {profileId !== '' && (
            <>
              <ProfileBar
                profiles={profiles}
                personas={[]}
                profileId={profileId}
                onSelect={select}
                personaHref={() => EDUCATION_PATH}
                personaLabel=""
              />
              <div className="mb-3 flex items-center justify-start gap-3">
                <button
                  type="button"
                  onClick={() => {
                    openEntry()
                  }}
                  className="flex shrink-0 items-center gap-1.5 rounded-md bg-gray-800 px-3.5 py-2 text-sm font-medium text-white transition-colors hover:bg-gray-900"
                >
                  <PlusIcon />
                  Add education
                </button>
                <p className="min-w-0 flex-1 text-xs text-gray-500">
                  An entry without an end date counts as still going on. Short trainings and certificates go on the Courses page.
                </p>
              </div>
              {entries.length === 0 && shownError === '' && (
                <div
                  className="career-wiggle rounded-3xl border p-8 text-center text-sm text-gray-500"
                  style={{ borderColor: PAGE_ACCENT.soft, background: PAGE_ACCENT.soft }}
                >
                  No education yet. Add the first entry with “Add education”.
                </div>
              )}
              <div className="career-stagger space-y-3">
                {entries.map((e) => (
                  <div
                    key={e.id}
                    className="rounded-3xl border p-4 shadow-sm transition-shadow hover:shadow-md"
                    style={{ borderColor: PAGE_ACCENT.soft, background: `linear-gradient(160deg, #ffffff 45%, ${PAGE_ACCENT.soft} 100%)` }}
                  >
                    <div className="flex items-start gap-3">
                      <span
                        className="inline-flex h-10 w-10 shrink-0 items-center justify-center rounded-full text-base font-bold text-white shadow-sm"
                        style={{ background: accentFor(e.institution).to }}
                        aria-hidden="true"
                      >
                        {initialOf(e.institution)}
                      </span>
                      <div className="min-w-0 flex-1">
                        <div className="text-sm font-semibold">{e.institution}</div>
                        {degreeLine(e) !== '' && <div className="text-xs text-gray-500">{degreeLine(e)}</div>}
                        <div className="mt-2 flex flex-wrap items-center gap-2">
                          <span
                            className="rounded-full px-2.5 py-0.5 text-xs font-medium"
                            style={{ background: PAGE_ACCENT.soft, color: PAGE_ACCENT.text }}
                          >
                            {periodText(e.startDate, e.endDate)}
                          </span>
                          {isOngoing(e) && (
                            <span className="rounded-full bg-green-50 px-2.5 py-0.5 text-xs font-medium text-green-700">Ongoing</span>
                          )}
                        </div>
                        {e.description !== '' && <div className="mt-2 whitespace-pre-wrap text-xs text-gray-600">{e.description}</div>}
                      </div>
                      <div className="flex shrink-0 gap-2">
                        <button
                          type="button"
                          onClick={() => {
                            openEntry(e.id)
                          }}
                          className="rounded-full border border-blue-200 px-3 py-1 text-xs text-blue-700 hover:bg-blue-50"
                        >
                          Edit
                        </button>
                        <button
                          type="button"
                          onClick={() => {
                            handleRemove(e.id)
                          }}
                          className="rounded-full border border-red-200 px-3 py-1 text-xs text-red-700 hover:bg-red-50"
                        >
                          Remove
                        </button>
                      </div>
                    </div>
                  </div>
                ))}
              </div>
            </>
          )}
        </div>
        <aside className="order-2 w-full shrink-0 lg:order-1 lg:sticky lg:top-6 lg:w-72">
          <PageTips page="education" orientation="vertical" />
        </aside>
      </div>
    </div>
  )
}
