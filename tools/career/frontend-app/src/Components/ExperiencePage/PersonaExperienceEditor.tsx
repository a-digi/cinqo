import { useEffect, useState } from 'react'
import { fetchExperience, removeExperience, type CareerExperience } from '../../api'
import { PlusIcon } from '../../Shared/Icons/icons'
import { EXPERIENCE_ENTRY_PATH } from '../../paths'
import { PAGE_ACCENT, accentFor, initialOf } from '../JobsPage/jobsTheme'
import { CareerTimeline } from './CareerTimeline'

// One persona's work history, editable: the career timeline, "Add experience" and the entries, each with Edit
// (both open ExperienceEntryPage) and Remove. Used by the persona's own experience page; onCount tells the page
// around it how many entries there are (for its header). See
// plan/ai/tools/career/step-07-dedicated-skills-and-experience-pages.md and step-120-experience-per-persona-pages.md.
export function PersonaExperienceEditor({ personaId, onCount }: { personaId: string; onCount?: (count: number) => void }) {
  const [experience, setExperience] = useState<CareerExperience[]>([])
  const [error, setError] = useState('')

  function load(forPersonaId: string) {
    setError('')
    fetchExperience(forPersonaId)
      .then(setExperience)
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
  }

  useEffect(() => {
    load(personaId)
  }, [personaId])

  function openEntry(forPersonaId: string, id?: string) {
    const idPart = id ? `&id=${encodeURIComponent(id)}` : ''
    window.__cinqoToolBridge.navigate(`${EXPERIENCE_ENTRY_PATH}?personaId=${encodeURIComponent(forPersonaId)}${idPart}`)
  }

  function handleRemove(id: string) {
    setError('')
    removeExperience(personaId, id)
      .then(() => {
        setExperience((prev) => prev.filter((e) => e.id !== id))
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
  }

  useEffect(() => {
    onCount?.(experience.length)
  }, [experience.length, onCount])

  return (
    <>
      <CareerTimeline entries={experience} today={new Date()} />

      <div className="mb-3 flex items-center justify-start gap-3">
        <button
          type="button"
          onClick={() => {
            openEntry(personaId)
          }}
          className="flex shrink-0 items-center gap-1.5 rounded-md bg-gray-800 px-3.5 py-2 text-sm font-medium text-white transition-colors hover:bg-gray-900"
        >
          <PlusIcon />
          Add experience
        </button>
        <p className="min-w-0 flex-1 text-xs text-gray-500">An entry without an end date counts as your current role.</p>
      </div>

      <div className="min-h-[1.2em] text-sm text-red-700">{error}</div>

      {experience.length === 0 && !error && (
        <div
          className="career-wiggle rounded-3xl border p-8 text-center text-sm text-gray-500"
          style={{ borderColor: PAGE_ACCENT.soft, background: PAGE_ACCENT.soft }}
        >
          No experience yet. Add the first entry with “Add experience”.
        </div>
      )}

      <div className="career-stagger space-y-3">
        {experience.map((exp) => {
          const isCurrent = exp.endDate.trim() === ''
          return (
            <div
              key={exp.id}
              className="rounded-3xl border p-4 shadow-sm transition-shadow hover:shadow-md"
              style={{
                borderColor: PAGE_ACCENT.soft,
                background: `linear-gradient(160deg, #ffffff 45%, ${PAGE_ACCENT.soft} 100%)`,
              }}
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
                      {exp.startDate || '—'} – {exp.endDate || 'present'}
                    </span>
                    {isCurrent && (
                      <span className="rounded-full bg-green-50 px-2.5 py-0.5 text-xs font-medium text-green-700">Current</span>
                    )}
                  </div>
                  {exp.description && <div className="mt-2 whitespace-pre-wrap text-xs text-gray-600">{exp.description}</div>}
                </div>
                <div className="flex shrink-0 gap-2">
                  <button
                    type="button"
                    onClick={() => {
                      openEntry(personaId, exp.id)
                    }}
                    className="rounded-full border border-blue-200 px-3 py-1 text-xs text-blue-700 hover:bg-blue-50"
                  >
                    Edit
                  </button>
                  <button
                    type="button"
                    onClick={() => {
                      handleRemove(exp.id)
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
