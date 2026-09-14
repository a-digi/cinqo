import { useEffect, useState } from 'react'
import { fetchProfile, updateProfile } from './api'
import { Field } from './Field'

const emptyForm = {
  fullName: '',
  headline: '',
  summary: '',
  location: '',
  desiredTitles: '',
  desiredLocations: '',
  minSalary: '',
}

// Core profile fields only — Skills and Experience moved to their own
// pages (SkillsPage / ExperiencePage) as of step 7, reversing step 5's
// own "inline for a first pass" call now that the request has made
// the split explicit. See
// plan/ai/tools/career/step-07-dedicated-skills-and-experience-pages.md.
export function ProfilePage() {
  const [form, setForm] = useState(emptyForm)
  const [error, setError] = useState('')
  const [saving, setSaving] = useState(false)

  function load() {
    setError('')
    fetchProfile()
      .then((result) => {
        setForm({
          fullName: result.profile?.fullName ?? '',
          headline: result.profile?.headline ?? '',
          summary: result.profile?.summary ?? '',
          location: result.profile?.location ?? '',
          desiredTitles: result.profile?.desiredTitles ?? '',
          desiredLocations: result.profile?.desiredLocations ?? '',
          minSalary: result.profile?.minSalary ? String(result.profile.minSalary) : '',
        })
      })
      .catch((err: Error) => setError(err.message))
  }

  useEffect(() => {
    load()
  }, [])

  function handleSave() {
    setError('')
    setSaving(true)
    updateProfile({
      fullName: form.fullName,
      headline: form.headline,
      summary: form.summary,
      location: form.location,
      desiredTitles: form.desiredTitles,
      desiredLocations: form.desiredLocations,
      minSalary: form.minSalary ? Number(form.minSalary) : undefined,
    })
      .catch((err: Error) => setError(err.message))
      .finally(() => setSaving(false))
  }

  return (
    <div className="max-w-2xl p-6 font-sans text-gray-900">
      <h1 className="mb-1.5 text-xl font-semibold">Career Profile</h1>
      <p className="mb-5 text-sm text-gray-500">
        Your own career profile — what the AI reads and updates when helping you find and apply
        for jobs. Nothing here is treated as a secret.
      </p>

      <div className="min-h-[1.2em] text-sm text-red-700">{error}</div>

      <section className="rounded-md border border-gray-200 p-4 shadow-sm">
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
          <Field label="Full name" value={form.fullName} onChange={(v) => setForm({ ...form, fullName: v })} />
          <Field label="Headline" value={form.headline} onChange={(v) => setForm({ ...form, headline: v })} />
          <Field label="Location" value={form.location} onChange={(v) => setForm({ ...form, location: v })} />
          <Field
            label="Minimum salary"
            type="number"
            value={form.minSalary}
            onChange={(v) => setForm({ ...form, minSalary: v })}
          />
          <Field
            label="Desired titles"
            value={form.desiredTitles}
            onChange={(v) => setForm({ ...form, desiredTitles: v })}
          />
          <Field
            label="Desired locations"
            value={form.desiredLocations}
            onChange={(v) => setForm({ ...form, desiredLocations: v })}
          />
        </div>
        <label className="mt-3 block text-xs font-medium text-gray-500">
          Summary
          <textarea
            value={form.summary}
            onChange={(e) => setForm({ ...form, summary: e.target.value })}
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
          {saving ? 'Saving…' : 'Save profile'}
        </button>
      </section>
    </div>
  )
}
