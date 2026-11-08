import { useEffect, useState } from 'react'
import { fetchPersonaDetails, updatePersonaDetails } from '../../../api'
import { Field } from '../../Field/Field'
import { PersonaSwitcher } from '../PersonaSwitcher/PersonaSwitcher'

const emptyForm = {
  headline: '',
  summary: '',
  location: '',
  desiredTitles: '',
  desiredLocations: '',
  minSalary: '',
}

// Renamed from ProfilePage.tsx as of step 11, matching the backend's
// own step 10 rename: this was never the job seeker's own profile
// (that's the new ProfilesPage/Profile entity) — it's a persona's own
// career positioning (headline/summary/location/desired titles/min
// salary). See
// plan/ai/tools/career/step-07-dedicated-skills-and-experience-pages.md,
// plan/ai/tools/career/step-09-persona-frontend.md, and
// plan/ai/tools/career/step-11-job-seeker-profile-frontend.md.
export function PersonaDetailsPage() {
  const [personaId, setPersonaId] = useState<string | null>(null)
  const [form, setForm] = useState(emptyForm)
  const [error, setError] = useState('')
  const [saving, setSaving] = useState(false)

  function load(forPersonaId: string) {
    setError('')
    fetchPersonaDetails(forPersonaId)
      .then((result) => {
        setForm({
          headline: result.personaDetails?.headline ?? '',
          summary: result.personaDetails?.summary ?? '',
          location: result.personaDetails?.location ?? '',
          desiredTitles: result.personaDetails?.desiredTitles ?? '',
          desiredLocations: result.personaDetails?.desiredLocations ?? '',
          minSalary: result.personaDetails?.minSalary ? String(result.personaDetails.minSalary) : '',
        })
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
  }

  useEffect(() => {
    if (personaId) load(personaId)
  }, [personaId])

  function handleSave() {
    if (!personaId) return
    setError('')
    setSaving(true)
    updatePersonaDetails(personaId, {
      headline: form.headline,
      summary: form.summary,
      location: form.location,
      desiredTitles: form.desiredTitles,
      desiredLocations: form.desiredLocations,
      minSalary: form.minSalary ? Number(form.minSalary) : undefined,
    })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
      .finally(() => {
        setSaving(false)
      })
  }

  return (
    <div className="max-w-2xl p-6 font-sans text-gray-900">
      <h1 className="mb-1.5 text-xl font-semibold">Persona Details</h1>
      <p className="mb-5 text-sm text-gray-500">
        This persona's own career positioning — what the AI reads and updates when helping you find and apply for jobs under this persona.
        Nothing here is treated as a secret.
      </p>

      <PersonaSwitcher personaId={personaId} onChange={setPersonaId} />

      <div className="min-h-[1.2em] text-sm text-red-700">{error}</div>

      {personaId && (
        <section className="rounded-md border border-gray-200 p-4 shadow-sm">
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <Field
              label="Headline"
              value={form.headline}
              onChange={(v) => {
                setForm({ ...form, headline: v })
              }}
            />
            <Field
              label="Location"
              value={form.location}
              onChange={(v) => {
                setForm({ ...form, location: v })
              }}
            />
            <Field
              label="Minimum salary"
              type="number"
              value={form.minSalary}
              onChange={(v) => {
                setForm({ ...form, minSalary: v })
              }}
            />
            <Field
              label="Desired titles"
              value={form.desiredTitles}
              onChange={(v) => {
                setForm({ ...form, desiredTitles: v })
              }}
            />
            <Field
              label="Desired locations"
              value={form.desiredLocations}
              onChange={(v) => {
                setForm({ ...form, desiredLocations: v })
              }}
            />
          </div>
          <label className="mt-3 block text-xs font-medium text-gray-500">
            Summary
            <textarea
              value={form.summary}
              onChange={(e) => {
                setForm({ ...form, summary: e.target.value })
              }}
              rows={3}
              className="mt-1 w-full rounded-md border border-gray-300 px-2.5 py-1.5 text-sm text-gray-900 focus:border-gray-500 focus:outline-none"
            />
          </label>
          <button
            type="button"
            onClick={handleSave}
            disabled={saving}
            className="mt-3 rounded-md bg-gray-900 px-3.5 py-2 text-sm font-medium text-white transition-colors hover:bg-gray-800 disabled:opacity-50"
          >
            {saving ? 'Saving…' : 'Save details'}
          </button>
        </section>
      )}
    </div>
  )
}
