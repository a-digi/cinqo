import { useEffect, useState } from 'react'
import { addExperience, fetchExperience, fetchPersonas, updateExperience } from '../../api'
import { PlusIcon, TimelineIcon } from '../../Shared/Icons/icons'
import { inputClass } from '../../Shared/inputClass'
import { PageHeaderCard } from '../../Shared/PageHeader/PageHeaderCard'
import { EXPERIENCE_PATH } from '../../paths'
import { Field } from '../Field/Field'
import { PAGE_ACCENT } from '../JobsPage/jobsTheme'
import { getPersonaIdFromLocalStorage, setPersonaIdInLocalStorage } from '../Persona/PersonaSwitcher/repositoryLocalStorage'

const emptyEntry = { company: '', title: '', startDate: '', endDate: '', description: '' }

// The page behind the Experience list's "Add experience" and each entry's "Edit":
// ?personaId= says whose experience (else the remembered persona) and ?id= which entry
// to edit (none = add). The persona cannot be changed here; the list's own persona bar
// does that. Menu-less, like the other create pages.
export function ExperienceEntryPage() {
  const params = new URLSearchParams(window.location.search)
  const entryId = params.get('id') ?? ''
  const [personaId, setPersonaId] = useState('')
  const [personaName, setPersonaName] = useState('')
  const [form, setForm] = useState(emptyEntry)
  const [message, setMessage] = useState('')
  const [error, setError] = useState('')
  const [isLoaded, setIsLoaded] = useState(false)
  const [isSaving, setIsSaving] = useState(false)

  useEffect(() => {
    const wanted = params.get('personaId') ?? getPersonaIdFromLocalStorage() ?? ''
    fetchPersonas()
      .then(async (personas) => {
        const persona = personas.find((p) => p.id === wanted)
        if (!persona) {
          setMessage(personas.length === 0 ? 'There are no personas yet.' : 'That persona was not found.')
          return
        }
        setPersonaId(persona.id)
        setPersonaName(persona.name)
        if (entryId) {
          const entry = (await fetchExperience(persona.id)).find((e) => e.id === entryId)
          if (!entry) {
            setMessage('That experience entry was not found.')
            return
          }
          setForm({
            company: entry.company,
            title: entry.title,
            startDate: entry.startDate,
            endDate: entry.endDate,
            description: entry.description,
          })
        }
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
      .finally(() => {
        setIsLoaded(true)
      })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  function backToList() {
    window.__cinqoToolBridge.navigate(EXPERIENCE_PATH)
  }

  function handleSave() {
    if (!form.company.trim() || !form.title.trim()) {
      setError('Company and title are both required')
      return
    }
    setError('')
    setIsSaving(true)
    const saved = entryId ? updateExperience(personaId, { id: entryId, ...form }) : addExperience(personaId, form)
    saved
      .then(() => {
        // The list shows the remembered persona, so it opens on the one just changed.
        setPersonaIdInLocalStorage(personaId)
        backToList()
      })
      .catch((err: unknown) => {
        setIsSaving(false)
        setError(err instanceof Error ? err.message : String(err))
      })
  }

  return (
    <div className="max-w-3xl p-6 font-sans text-gray-900">
      <PageHeaderCard
        icon={<TimelineIcon />}
        title={entryId ? 'Edit experience' : 'Add experience'}
        description={personaName ? `A job or role on the persona “${personaName}”.` : 'A job or role on a persona.'}
      />
      {!isLoaded ? (
        <p className="text-sm text-gray-500">Loading…</p>
      ) : message ? (
        <div
          className="rounded-3xl border p-6 text-sm text-gray-700"
          style={{ borderColor: PAGE_ACCENT.soft, background: PAGE_ACCENT.soft }}
        >
          {message}{' '}
          <a
            href={EXPERIENCE_PATH}
            onClick={(e) => {
              e.preventDefault()
              backToList()
            }}
            className="font-medium underline"
          >
            Back to experience
          </a>
        </div>
      ) : (
        <form
          onSubmit={(e) => {
            e.preventDefault()
            handleSave()
          }}
          className="career-enter career-delay-1 rounded-3xl border p-5 shadow-sm"
          style={{ borderColor: PAGE_ACCENT.soft, background: `linear-gradient(160deg, #ffffff 45%, ${PAGE_ACCENT.soft} 100%)` }}
        >
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <Field
              label="Company"
              value={form.company}
              onChange={(v) => {
                setForm({ ...form, company: v })
              }}
            />
            <Field
              label="Title"
              value={form.title}
              onChange={(v) => {
                setForm({ ...form, title: v })
              }}
            />
            <Field
              label="Start date"
              value={form.startDate}
              onChange={(v) => {
                setForm({ ...form, startDate: v })
              }}
            />
            <Field
              label="End date (empty = present)"
              value={form.endDate}
              onChange={(v) => {
                setForm({ ...form, endDate: v })
              }}
            />
          </div>
          <label className="mt-3 block text-xs font-medium text-gray-500">
            Description
            <textarea
              value={form.description}
              onChange={(e) => {
                setForm({ ...form, description: e.target.value })
              }}
              rows={4}
              className={`mt-1 ${inputClass}`}
            />
          </label>
          <div className="min-h-[1.2em] pt-3 text-sm text-red-700">{error}</div>
          <div className="mt-3 flex gap-2">
            <button
              type="submit"
              disabled={isSaving || !isLoaded || !personaId}
              className="flex items-center gap-1 rounded-md bg-gray-900 px-3.5 py-2 text-sm font-medium text-white transition-colors hover:bg-gray-800 disabled:opacity-50"
            >
              {!entryId && <PlusIcon />}
              {entryId ? 'Save changes' : 'Add experience'}
            </button>
            <button
              type="button"
              onClick={backToList}
              className="rounded-md border border-gray-200 px-3.5 py-2 text-sm text-gray-700 hover:bg-gray-50"
            >
              Cancel
            </button>
          </div>
        </form>
      )}
    </div>
  )
}
