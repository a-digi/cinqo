import { useEffect, useState } from 'react'
import { fetchProfileExperience, removeProfileExperience, type ProfileExperience } from '../../api'
import { PlusIcon } from '../../Shared/Icons/icons'
import { EXPERIENCE_ENTRY_PATH } from '../../paths'
import { PAGE_ACCENT, accentFor, initialOf } from '../JobsPage/jobsTheme'
import { dateText, removeWarning, timelineEntries, usedByText } from '../ProfileLibrary/libraryRules'
import { CareerTimeline } from './CareerTimeline'

// The profile's library of work experience (plan 173): the career timeline, "Add experience" and the entries, each with Edit (opens
// ExperienceEntryPage on the library entry) and Remove (an entry personas use asks first). onCount tells the page around it how many
// entries there are (for its header).
export function ProfileExperienceEditor({ profileId, onCount }: { profileId: string; onCount?: (count: number) => void }) {
  const [experience, setExperience] = useState<ProfileExperience[]>([])
  const [confirming, setConfirming] = useState<string | null>(null)
  const [error, setError] = useState('')

  useEffect(() => {
    setError('')
    setConfirming(null)
    fetchProfileExperience(profileId)
      .then(setExperience)
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
  }, [profileId])

  function openEntry(id?: string) {
    const idPart = id ? `&id=${encodeURIComponent(id)}` : ''
    window.__cinqoToolBridge.navigate(`${EXPERIENCE_ENTRY_PATH}?profileId=${encodeURIComponent(profileId)}${idPart}`)
  }

  function doRemove(entry: ProfileExperience) {
    setError('')
    setConfirming(null)
    removeProfileExperience(entry.id)
      .then(() => {
        setExperience((prev) => prev.filter((e) => e.id !== entry.id))
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
  }

  function askRemove(entry: ProfileExperience) {
    if (removeWarning(entry.usedBy) === null) doRemove(entry)
    else setConfirming(entry.id)
  }

  useEffect(() => {
    onCount?.(experience.length)
  }, [experience.length, onCount])

  const pending = experience.find((e) => e.id === confirming)

  return (
    <>
      <CareerTimeline entries={timelineEntries(experience)} today={new Date()} />

      <div className="mb-3 flex items-center justify-start gap-3">
        <button
          type="button"
          onClick={() => {
            openEntry()
          }}
          className="flex shrink-0 items-center gap-1.5 rounded-md bg-gray-800 px-3.5 py-2 text-sm font-medium text-white transition-colors hover:bg-gray-900"
        >
          <PlusIcon />
          Add experience
        </button>
        <p className="min-w-0 flex-1 text-xs text-gray-500">
          An entry without an end date counts as your current role. Each target role chooses its entries from this library.
        </p>
      </div>

      <div className="min-h-[1.2em] text-sm text-red-700">{error}</div>

      {pending && (
        <div
          role="alert"
          className="mb-3 flex flex-wrap items-center gap-2 rounded-2xl border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-900"
        >
          <span className="min-w-0 flex-1">
            <strong>
              {pending.title} at {pending.company}
            </strong>
            : {removeWarning(pending.usedBy)}
          </span>
          <button
            type="button"
            onClick={() => {
              doRemove(pending)
            }}
            className="rounded-md bg-red-700 px-2.5 py-1 font-medium text-white hover:bg-red-800"
          >
            Remove
          </button>
          <button
            type="button"
            onClick={() => {
              setConfirming(null)
            }}
            className="rounded-md border border-gray-300 bg-white px-2.5 py-1 text-gray-700 hover:bg-gray-50"
          >
            Keep
          </button>
        </div>
      )}

      {experience.length === 0 && !error && (
        <div
          className="career-wiggle rounded-3xl border p-8 text-center text-sm text-gray-500"
          style={{ borderColor: PAGE_ACCENT.soft, background: PAGE_ACCENT.soft }}
        >
          No experience in the library yet. Add the first entry with “Add experience”.
        </div>
      )}

      <div className="career-stagger space-y-3">
        {experience.map((exp) => {
          const isCurrent = (exp.endDate ?? '').trim() === ''
          return (
            <div
              key={exp.id}
              className="rounded-3xl border p-4 shadow-sm transition-shadow hover:shadow-md"
              style={{ borderColor: PAGE_ACCENT.soft, background: `linear-gradient(160deg, #ffffff 45%, ${PAGE_ACCENT.soft} 100%)` }}
            >
              <div className="flex items-start gap-3">
                <span
                  className="inline-flex h-10 w-10 shrink-0 items-center justify-center rounded-full text-base font-bold text-white shadow-sm"
                  style={{ background: accentFor(exp.company).to }}
                  aria-hidden="true"
                >
                  {initialOf(exp.company)}
                </span>
                <div className="min-w-0 flex-1">
                  <div className="text-sm font-semibold">{exp.title}</div>
                  <div className="text-xs text-gray-500">{exp.company}</div>
                  <div className="mt-2 flex flex-wrap items-center gap-2">
                    <span
                      className="rounded-full px-2.5 py-0.5 text-xs font-medium"
                      style={{ background: PAGE_ACCENT.soft, color: PAGE_ACCENT.text }}
                    >
                      {dateText(exp.startDate, '—')} – {dateText(exp.endDate, 'present')}
                    </span>
                    {isCurrent && (
                      <span className="rounded-full bg-green-50 px-2.5 py-0.5 text-xs font-medium text-green-700">Current</span>
                    )}
                    <span className="rounded-full bg-gray-100 px-2.5 py-0.5 text-xs text-gray-600">{usedByText(exp.usedBy)}</span>
                  </div>
                  {exp.description && <div className="mt-2 whitespace-pre-wrap text-xs text-gray-600">{exp.description}</div>}
                </div>
                <div className="flex shrink-0 gap-2">
                  <button
                    type="button"
                    onClick={() => {
                      openEntry(exp.id)
                    }}
                    className="rounded-full border border-blue-200 px-3 py-1 text-xs text-blue-700 hover:bg-blue-50"
                  >
                    Edit
                  </button>
                  <button
                    type="button"
                    onClick={() => {
                      askRemove(exp)
                    }}
                    className="rounded-full border border-red-200 px-3 py-1 text-xs text-red-700 hover:bg-red-50"
                  >
                    Remove
                  </button>
                </div>
              </div>
            </div>
          )
        })}
      </div>
    </>
  )
}
