import { useState } from 'react'
import type { CvExperienceEntry } from '../../api'
import { PlusIcon, XIcon } from '../../Shared/Icons/icons'
import { ExperienceForm } from './ExperienceForm'
import type { CvStepProps } from './PersonalDetailsStep'

const emptyExperience: CvExperienceEntry = { company: '', title: '', startDate: '', endDate: '', description: '' }

// Wizard step 4. See PersonalDetailsStep's own doc comment for the
// shared "local CvData only" contract every step here follows, and
// ExperienceForm's own doc comment for the "adding new" render bug
// this component's own earlier, single-page version had (fixed here
// from the start — the "adding new" case has its own explicit render
// branch below, not nested inside the existing-entries map).
export function ExperienceStep({ data, onChange }: CvStepProps) {
  const [editingIndex, setEditingIndex] = useState<number | null>(null)
  const [form, setForm] = useState<CvExperienceEntry>(emptyExperience)

  const startAdd = () => {
    setEditingIndex(data.experience.length)
    setForm(emptyExperience)
  }

  const startEdit = (index: number) => {
    setEditingIndex(index)
    setForm(data.experience[index])
  }

  const cancelEdit = () => {
    setEditingIndex(null)
    setForm(emptyExperience)
  }

  const save = () => {
    if (editingIndex === null) return
    const next = [...data.experience]
    if (editingIndex < next.length) {
      next[editingIndex] = form
    } else {
      next.push(form)
    }
    onChange({ ...data, experience: next })
    cancelEdit()
  }

  const remove = (index: number) => {
    onChange({ ...data, experience: data.experience.filter((_, i) => i !== index) })
    if (editingIndex === index) cancelEdit()
  }

  return (
    <div>
      <div className="space-y-2">
        {data.experience.map((entry, i) =>
          editingIndex === i ? (
            <ExperienceForm key={i} value={form} onChange={setForm} onSave={save} onCancel={cancelEdit} />
          ) : (
            <div key={i} className="flex items-start justify-between gap-2 rounded-md border border-gray-200 p-2">
              <button
                type="button"
                onClick={() => {
                  startEdit(i)
                }}
                className="min-w-0 flex-1 text-left"
              >
                <p className="truncate text-sm font-medium text-gray-900">
                  {entry.title || 'Untitled'}
                  {entry.company ? ` — ${entry.company}` : ''}
                </p>
                <p className="text-xs text-gray-500">
                  {entry.startDate} – {entry.endDate || 'Present'}
                </p>
              </button>
              <button
                type="button"
                onClick={() => {
                  remove(i)
                }}
                className="shrink-0 text-gray-400 hover:text-red-600"
                aria-label="Remove experience entry"
              >
                <XIcon />
              </button>
            </div>
          ),
        )}
        {editingIndex === data.experience.length && <ExperienceForm value={form} onChange={setForm} onSave={save} onCancel={cancelEdit} />}
      </div>
      {editingIndex === null && (
        <button
          type="button"
          onClick={startAdd}
          className="mt-2 flex items-center gap-1 text-xs font-medium text-gray-700 hover:text-gray-900"
        >
          <PlusIcon /> Add experience
        </button>
      )}
    </div>
  )
}
