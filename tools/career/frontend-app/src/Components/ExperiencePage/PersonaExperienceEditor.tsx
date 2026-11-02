import { useEffect, useState } from 'react'
import {
  fetchPersonaExperienceView,
  removeExperience,
  setPersonaExperienceUse,
  type CareerExperience,
  type ExperienceUse,
  type PersonaExperienceView,
  type ProfileExperience,
} from '../../api'
import { PlusIcon } from '../../Shared/Icons/icons'
import { inputClass } from '../../Shared/inputClass'
import { EXPERIENCE_ENTRY_PATH, EXPERIENCE_PATH } from '../../paths'
import { PAGE_ACCENT, accentFor, initialOf } from '../JobsPage/jobsTheme'
import { buildProfileSearch, dateText, timelineEntries } from '../ProfileLibrary/libraryRules'
import {
  hasWording,
  toggleUse,
  usesText,
  withWording,
  withoutDraft,
  wordingOf,
  wordingProblem,
  type Wording,
} from '../ProfileLibrary/personaUseRules'
import { CareerTimeline } from './CareerTimeline'

const cardStyle = { borderColor: PAGE_ACCENT.soft, background: `linear-gradient(160deg, #ffffff 45%, ${PAGE_ACCENT.soft} 100%)` }

// One persona's work history (plan 173): the entries of the profile's library it USES (a checklist; for a used entry the persona may give
// its own title and description for the role it represents, and reset them to the profile's), and the entries that belong only to this
// persona (add, edit, remove). The career timeline above shows both. onCount tells the page around it how many entries the persona shows.
export function PersonaExperienceEditor({
  personaId,
  profileId,
  onCount,
}: {
  personaId: string
  profileId: string
  onCount?: (count: number) => void
}) {
  const [view, setView] = useState<PersonaExperienceView | null>(null)
  // The wording being edited, by library entry id; an entry without a draft shows what is saved.
  const [drafts, setDrafts] = useState<Partial<Record<string, Wording>>>({})
  const [error, setError] = useState('')

  function load() {
    setError('')
    fetchPersonaExperienceView(personaId)
      .then(setView)
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
  }

  useEffect(() => {
    setDrafts({})
    load()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [personaId])

  function fail(err: unknown) {
    setError(err instanceof Error ? err.message : String(err))
  }

  function save(uses: ExperienceUse[], clearDraft?: string) {
    setError('')
    setPersonaExperienceUse(personaId, uses)
      .then((next) => {
        setView(next)
        if (clearDraft !== undefined) setDrafts((prev) => withoutDraft(prev, clearDraft))
      })
      .catch(fail)
  }

  function openEntry(id?: string) {
    const idPart = id ? `&id=${encodeURIComponent(id)}` : ''
    window.__cinqoToolBridge.navigate(`${EXPERIENCE_ENTRY_PATH}?personaId=${encodeURIComponent(personaId)}${idPart}`)
  }

  function removeOwn(id: string) {
    setError('')
    removeExperience(personaId, id).then(load).catch(fail)
  }

  const used = view?.used ?? []
  const library = view?.library ?? []
  const own = view?.own ?? []
  const usedLibrary = library.filter((e) => used.some((u) => u.id === e.id))

  useEffect(() => {
    if (view) onCount?.(view.used.length + view.own.length)
  }, [view, onCount])

  // The timeline shows what the persona shows: the library entries it uses, and its own.
  const timeline = [
    ...timelineEntries(usedLibrary).map((e, i) => ({
      ...e,
      title: wordingOf(
        used.find((u) => u.id === usedLibrary[i].id),
        usedLibrary[i],
      ).title,
    })),
    ...own.map((e) => ({ ...e })),
  ]

  function libraryCard(entry: ProfileExperience) {
    const use = used.find((u) => u.id === entry.id)
    const on = use !== undefined
    const draft = drafts[entry.id]
    const wording = wordingOf(use, entry)
    const problem = draft ? wordingProblem(draft) : ''
    return (
      <div key={entry.id} className="rounded-3xl border p-4 shadow-sm" style={{ ...cardStyle, opacity: on ? 1 : 0.8 }}>
        <div className="flex items-start gap-3">
          <input
            type="checkbox"
            checked={on}
            aria-label={`Use ${entry.title} at ${entry.company} on this persona`}
            onChange={(e) => {
              save(toggleUse(used, entry.id, e.target.checked), entry.id)
            }}
            className="mt-1 h-4 w-4 shrink-0"
          />
          <span
            className="inline-flex h-10 w-10 shrink-0 items-center justify-center rounded-full text-base font-bold text-white shadow-sm"
            style={{ background: accentFor(entry.company).to }}
            aria-hidden="true"
          >
            {initialOf(entry.company)}
          </span>
          <div className="min-w-0 flex-1">
            <div className="text-sm font-semibold">{on ? wording.title : entry.title}</div>
            <div className="text-xs text-gray-500">{entry.company}</div>
            <div className="mt-2 flex flex-wrap items-center gap-2">
              <span
                className="rounded-full px-2.5 py-0.5 text-xs font-medium"
                style={{ background: PAGE_ACCENT.soft, color: PAGE_ACCENT.text }}
              >
                {dateText(entry.startDate, '—')} – {dateText(entry.endDate, 'present')}
              </span>
              {hasWording(use) && (
                <span className="rounded-full bg-violet-50 px-2.5 py-0.5 text-xs font-medium text-violet-700">Own wording</span>
              )}
            </div>
            {!draft && (on ? wording.description : entry.description) && (
              <div className="mt-2 whitespace-pre-wrap text-xs text-gray-600">{on ? wording.description : entry.description}</div>
            )}
            {on && !draft && (
              <div className="mt-2 flex gap-2">
                <button
                  type="button"
                  onClick={() => {
                    setDrafts({ ...drafts, [entry.id]: { ...wording, on: true } })
                  }}
                  className="rounded-full border border-blue-200 px-3 py-1 text-xs text-blue-700 hover:bg-blue-50"
                >
                  {hasWording(use) ? 'Edit the wording' : 'Word it for this role'}
                </button>
                {hasWording(use) && (
                  <button
                    type="button"
                    onClick={() => {
                      save(
                        used.map((u) => (u.id === entry.id ? { id: entry.id } : u)),
                        entry.id,
                      )
                    }}
                    className="rounded-full border border-gray-200 px-3 py-1 text-xs text-gray-700 hover:bg-gray-50"
                  >
                    Use the profile’s text
                  </button>
                )}
              </div>
            )}
            {on && draft && (
              <div className="mt-3 space-y-2">
                <label className="block text-xs font-medium text-gray-500">
                  Title for this persona
                  <input
                    value={draft.title}
                    onChange={(e) => {
                      setDrafts({ ...drafts, [entry.id]: { ...draft, title: e.target.value } })
                    }}
                    className={`mt-1 ${inputClass}`}
                  />
                </label>
                <label className="block text-xs font-medium text-gray-500">
                  Description for this persona
                  <textarea
                    value={draft.description}
                    onChange={(e) => {
                      setDrafts({ ...drafts, [entry.id]: { ...draft, description: e.target.value } })
                    }}
                    rows={4}
                    className={`mt-1 ${inputClass}`}
                  />
                </label>
                {problem !== '' && <p className="text-xs text-red-700">{problem}</p>}
                <div className="flex gap-2">
                  <button
                    type="button"
                    disabled={problem !== ''}
                    onClick={() => {
                      save(withWording(used, entry.id, draft), entry.id)
                    }}
                    className="rounded-md bg-gray-900 px-3 py-1 text-xs font-medium text-white hover:bg-gray-800 disabled:opacity-50"
                  >
                    Save wording
                  </button>
                  <button
                    type="button"
                    onClick={() => {
                      setDrafts((prev) => withoutDraft(prev, entry.id))
                    }}
                    className="rounded-md border border-gray-200 px-3 py-1 text-xs text-gray-700 hover:bg-gray-50"
                  >
                    Cancel
                  </button>
                </div>
              </div>
            )}
          </div>
        </div>
      </div>
    )
  }

  function ownCard(exp: CareerExperience) {
    const isCurrent = exp.endDate.trim() === ''
    return (
      <div key={exp.id} className="rounded-3xl border border-dashed p-4 shadow-sm" style={cardStyle}>
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
              {isCurrent && <span className="rounded-full bg-green-50 px-2.5 py-0.5 text-xs font-medium text-green-700">Current</span>}
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
                removeOwn(exp.id)
              }}
              className="rounded-full border border-red-200 px-3 py-1 text-xs text-red-700 hover:bg-red-50"
            >
              Remove
            </button>
          </div>
        </div>
      </div>
    )
  }

  const libraryHref = `${EXPERIENCE_PATH}${buildProfileSearch(profileId)}`

  return (
    <>
      <CareerTimeline entries={timeline} today={new Date()} />

      <div className="min-h-[1.2em] text-sm text-red-700">{error}</div>

      <section className="mb-6">
        <div className="mb-3 flex flex-wrap items-baseline justify-between gap-2">
          <h2 className="text-sm font-semibold text-gray-800">From the profile’s library</h2>
          {view && <span className="text-xs text-gray-500">{usesText(used.length, library.length, 'entry', 'entries')}</span>}
        </div>
        {view?.library.length === 0 && (
          <p className="text-sm text-gray-500">
            The profile has no experience yet.{' '}
            <a
              href={libraryHref}
              onClick={(e) => {
                e.preventDefault()
                window.__cinqoToolBridge.navigate(libraryHref)
              }}
              className="font-medium text-blue-700 hover:underline"
            >
              Add entries to the library
            </a>{' '}
            and choose here which ones this persona uses.
          </p>
        )}
        <div className="career-stagger space-y-3">{library.map(libraryCard)}</div>
      </section>

      <section>
        <div className="mb-3 flex flex-wrap items-center justify-between gap-3">
          <h2 className="text-sm font-semibold text-gray-800">Only on this persona</h2>
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
        </div>
        {own.length === 0 && (
          <p className="mb-3 text-sm text-gray-500">
            No entries of its own yet. Add one if this role needs an entry the profile does not list.
          </p>
        )}
        <div className="career-stagger space-y-3">{own.map(ownCard)}</div>
      </section>
    </>
  )
}
