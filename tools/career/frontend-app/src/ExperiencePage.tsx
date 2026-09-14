import { useEffect, useState } from 'react'
import { fetchExperience, addExperience, updateExperience, removeExperience, type CareerExperience } from './api'
import { Field } from './Field'
import { PlusIcon } from './icons'

const emptyEntry = { company: '', title: '', startDate: '', endDate: '', description: '' }

// Its own page as of step 7 (moved out of ProfilePage), now with a
// real edit-in-place capability the inline version never had —
// clicking "Edit" on an entry loads it into the same form used for
// adding and switches the submit action to update_career_experience
// via updateExperience(), a genuinely new backend operation (see
// plan/ai/tools/career/step-07-dedicated-skills-and-experience-pages.md).
export function ExperiencePage() {
  const [experience, setExperience] = useState<CareerExperience[]>([])
  const [form, setForm] = useState(emptyEntry)
  const [editingId, setEditingId] = useState<string | null>(null)
  const [error, setError] = useState('')

  function load() {
    setError('')
    fetchExperience()
      .then(setExperience)
      .catch((err: Error) => setError(err.message))
  }

  useEffect(() => {
    load()
  }, [])

  function startEdit(entry: CareerExperience) {
    setEditingId(entry.id)
    setForm({
      company: entry.company,
      title: entry.title,
      startDate: entry.startDate,
      endDate: entry.endDate,
      description: entry.description,
    })
  }

  function cancelEdit() {
    setEditingId(null)
    setForm(emptyEntry)
  }

  function handleSubmit() {
    if (!form.company.trim() || !form.title.trim()) {
      setError('Company and title are both required')
      return
    }
    setError('')
    if (editingId) {
      updateExperience({ id: editingId, ...form })
        .then((result) => {
          setExperience(result)
          cancelEdit()
        })
        .catch((err: Error) => setError(err.message))
    } else {
      addExperience(form)
        .then((result) => {
          setExperience(result)
          setForm(emptyEntry)
        })
        .catch((err: Error) => setError(err.message))
    }
  }

  function handleRemove(id: string) {
    setError('')
    removeExperience(id)
      .then(() => {
        setExperience((prev) => prev.filter((e) => e.id !== id))
        if (editingId === id) cancelEdit()
      })
      .catch((err: Error) => setError(err.message))
  }

  return (
    <div className="max-w-2xl p-6 font-sans text-gray-900">
      <h1 className="mb-1.5 text-xl font-semibold">Experience</h1>
      <p className="mb-5 text-sm text-gray-500">
        Work experience on your own career profile — what the AI reads when matching you against
        job postings.
      </p>

      <div className="min-h-[1.2em] text-sm text-red-700">{error}</div>

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
            <div className="flex shrink-0 gap-2">
              <button
                type="button"
                onClick={() => startEdit(exp)}
                className="rounded-md border border-gray-200 px-2.5 py-1 text-xs text-gray-700 hover:bg-gray-50"
              >
                Edit
              </button>
              <button
                type="button"
                onClick={() => handleRemove(exp.id)}
                className="rounded-md border border-gray-200 px-2.5 py-1 text-xs text-red-700 hover:bg-red-50"
              >
                Remove
              </button>
            </div>
          </div>
        ))}
      </div>

      <section className="rounded-md border border-gray-200 p-4 shadow-sm">
        <h2 className="mb-3 text-sm font-semibold">{editingId ? 'Edit entry' : 'Add entry'}</h2>
        <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
          <Field label="Company" value={form.company} onChange={(v) => setForm({ ...form, company: v })} />
          <Field label="Title" value={form.title} onChange={(v) => setForm({ ...form, title: v })} />
          <Field label="Start date" value={form.startDate} onChange={(v) => setForm({ ...form, startDate: v })} />
          <Field label="End date" value={form.endDate} onChange={(v) => setForm({ ...form, endDate: v })} />
        </div>
        <label className="mt-2 block text-xs font-medium text-gray-500">
          Description
          <textarea
            value={form.description}
            onChange={(e) => setForm({ ...form, description: e.target.value })}
            rows={2}
            className="mt-1 w-full rounded-md border border-gray-300 px-2.5 py-1.5 text-sm text-gray-900 focus:border-gray-500 focus:outline-none"
          />
        </label>
        <div className="mt-2 flex gap-2">
          <button
            type="button"
            onClick={handleSubmit}
            className="flex items-center gap-1 rounded-md bg-gray-900 px-3.5 py-2 text-sm font-medium text-white transition-colors hover:bg-gray-800"
          >
            {!editingId && <PlusIcon />}
            {editingId ? 'Save changes' : 'Add experience'}
          </button>
          {editingId && (
            <button
              type="button"
              onClick={cancelEdit}
              className="rounded-md border border-gray-200 px-3.5 py-2 text-sm text-gray-700 hover:bg-gray-50"
            >
              Cancel
            </button>
          )}
        </div>
      </section>
    </div>
  )
}
