import { useEffect, useState } from 'react'
import {
  fetchProfile,
  updateProfile,
  addSkill,
  removeSkill,
  addExperience,
  removeExperience,
  type CareerExperience,
} from './api'
import { PlusIcon, XIcon } from './icons'

const emptyForm = {
  fullName: '',
  headline: '',
  summary: '',
  location: '',
  desiredTitles: '',
  desiredLocations: '',
  minSalary: '',
}

// ProfilePage keeps skills/experience inline on one page rather than
// their own sub-views — a handful of chips/entries fits comfortably
// here, and unlike browser's own credentials flow there's no
// plaintext-secret-lingering-in-a-shared-view reason to isolate them.
// See this step's own open question 1.
export function ProfilePage() {
  const [form, setForm] = useState(emptyForm)
  const [skills, setSkills] = useState<string[]>([])
  const [experience, setExperience] = useState<CareerExperience[]>([])
  const [newSkill, setNewSkill] = useState('')
  const [newExp, setNewExp] = useState({ company: '', title: '', startDate: '', endDate: '', description: '' })
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
        setSkills(result.skills)
        setExperience(result.experience)
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
      .then((result) => {
        setSkills(result.skills)
        setExperience(result.experience)
      })
      .catch((err: Error) => setError(err.message))
      .finally(() => setSaving(false))
  }

  function handleAddSkill() {
    const skill = newSkill.trim()
    if (!skill) return
    setError('')
    addSkill(skill)
      .then((result) => {
        setSkills(result)
        setNewSkill('')
      })
      .catch((err: Error) => setError(err.message))
  }

  function handleRemoveSkill(skill: string) {
    setError('')
    removeSkill(skill)
      .then(() => setSkills((prev) => prev.filter((s) => s !== skill)))
      .catch((err: Error) => setError(err.message))
  }

  function handleAddExperience() {
    if (!newExp.company.trim() || !newExp.title.trim()) {
      setError('Company and title are both required')
      return
    }
    setError('')
    addExperience(newExp)
      .then((result) => {
        setExperience(result)
        setNewExp({ company: '', title: '', startDate: '', endDate: '', description: '' })
      })
      .catch((err: Error) => setError(err.message))
  }

  function handleRemoveExperience(id: string) {
    setError('')
    removeExperience(id)
      .then(() => setExperience((prev) => prev.filter((e) => e.id !== id)))
      .catch((err: Error) => setError(err.message))
  }

  return (
    <div className="max-w-2xl p-6 font-sans text-gray-900">
      <h1 className="mb-1.5 text-xl font-semibold">Career Profile</h1>
      <p className="mb-5 text-sm text-gray-500">
        Your own career profile — what the AI reads and updates when helping you find and apply
        for jobs. Nothing here is treated as a secret.
      </p>

      <div className="min-h-[1.2em] text-sm text-red-700">{error}</div>

      <section className="mb-6 rounded-md border border-gray-200 p-4 shadow-sm">
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

      <section className="mb-6 rounded-md border border-gray-200 p-4 shadow-sm">
        <h2 className="mb-3 text-sm font-semibold">Skills</h2>
        <div className="mb-3 flex flex-wrap gap-2">
          {skills.map((skill) => (
            <span
              key={skill}
              className="flex items-center gap-1.5 rounded-full bg-gray-100 px-3 py-1 text-xs text-gray-700"
            >
              {skill}
              <button
                type="button"
                aria-label={`Remove ${skill}`}
                onClick={() => handleRemoveSkill(skill)}
                className="text-gray-400 hover:text-red-700"
              >
                <XIcon />
              </button>
            </span>
          ))}
        </div>
        <div className="flex gap-2">
          <input
            value={newSkill}
            onChange={(e) => setNewSkill(e.target.value)}
            onKeyDown={(e) => e.key === 'Enter' && handleAddSkill()}
            placeholder="Add a skill"
            className="flex-1 rounded-md border border-gray-300 px-2.5 py-1.5 text-sm text-gray-900 focus:border-gray-500 focus:outline-none"
          />
          <button
            type="button"
            onClick={handleAddSkill}
            className="flex items-center gap-1 rounded-md border border-gray-200 px-3 py-1.5 text-sm text-gray-700 hover:bg-gray-50"
          >
            <PlusIcon />
            Add
          </button>
        </div>
      </section>

      <section className="rounded-md border border-gray-200 p-4 shadow-sm">
        <h2 className="mb-3 text-sm font-semibold">Experience</h2>
        <div className="mb-4 space-y-2">
          {experience.map((exp) => (
            <div key={exp.id} className="flex items-start justify-between gap-3 rounded-md border border-gray-200 p-3">
              <div>
                <div className="text-sm font-medium">{exp.title} · {exp.company}</div>
                <div className="text-xs text-gray-500">
                  {exp.startDate || '—'} – {exp.endDate || 'present'}
                </div>
                {exp.description && <div className="mt-1 text-xs text-gray-600">{exp.description}</div>}
              </div>
              <button
                type="button"
                onClick={() => handleRemoveExperience(exp.id)}
                className="shrink-0 rounded-md border border-gray-200 px-2.5 py-1 text-xs text-red-700 hover:bg-red-50"
              >
                Remove
              </button>
            </div>
          ))}
        </div>
        <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
          <Field label="Company" value={newExp.company} onChange={(v) => setNewExp({ ...newExp, company: v })} />
          <Field label="Title" value={newExp.title} onChange={(v) => setNewExp({ ...newExp, title: v })} />
          <Field label="Start date" value={newExp.startDate} onChange={(v) => setNewExp({ ...newExp, startDate: v })} />
          <Field label="End date" value={newExp.endDate} onChange={(v) => setNewExp({ ...newExp, endDate: v })} />
        </div>
        <label className="mt-2 block text-xs font-medium text-gray-500">
          Description
          <textarea
            value={newExp.description}
            onChange={(e) => setNewExp({ ...newExp, description: e.target.value })}
            rows={2}
            className="mt-1 w-full rounded-md border border-gray-300 px-2.5 py-1.5 text-sm text-gray-900 focus:border-gray-500 focus:outline-none"
          />
        </label>
        <button
          type="button"
          onClick={handleAddExperience}
          className="mt-2 flex items-center gap-1 rounded-md border border-gray-200 px-3 py-1.5 text-sm text-gray-700 hover:bg-gray-50"
        >
          <PlusIcon />
          Add experience
        </button>
      </section>
    </div>
  )
}

function Field({
  label,
  value,
  onChange,
  type = 'text',
}: {
  label: string
  value: string
  onChange: (v: string) => void
  type?: string
}) {
  return (
    <label className="block text-xs font-medium text-gray-500">
      {label}
      <input
        type={type}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        className="mt-1 w-full rounded-md border border-gray-300 px-2.5 py-1.5 text-sm text-gray-900 focus:border-gray-500 focus:outline-none"
      />
    </label>
  )
}
