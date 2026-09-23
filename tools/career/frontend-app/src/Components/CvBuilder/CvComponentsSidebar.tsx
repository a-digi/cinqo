import { useState } from 'react'
import type { CvData, CvExperienceEntry } from '../../api'
import { Field } from '../Field/Field'
import { PlusIcon, XIcon } from '../../Shared/Icons/icons'

const emptyExperience: CvExperienceEntry = { company: '', title: '', startDate: '', endDate: '', description: '' }

// The add/edit form for one experience entry — shared by BOTH the
// "editing an existing entry" and "adding a new one" cases below,
// which previously (bug, caught by live testing) only rendered inside
// data.experience.map(...), so "adding new" (editingExperienceIndex ===
// data.experience.length, one PAST the last real index) never matched
// any array element and silently rendered nothing at all.
function ExperienceForm({
  value,
  onChange,
  onSave,
  onCancel,
}: {
  value: CvExperienceEntry
  onChange: (updated: CvExperienceEntry) => void
  onSave: () => void
  onCancel: () => void
}) {
  return (
    <div className="space-y-2 rounded-md border border-gray-200 p-2">
      <Field
        label="Company"
        value={value.company}
        onChange={(v) => {
          onChange({ ...value, company: v })
        }}
      />
      <Field
        label="Title"
        value={value.title}
        onChange={(v) => {
          onChange({ ...value, title: v })
        }}
      />
      <div className="flex gap-2">
        <Field
          label="Start"
          value={value.startDate}
          onChange={(v) => {
            onChange({ ...value, startDate: v })
          }}
        />
        <Field
          label="End"
          value={value.endDate}
          onChange={(v) => {
            onChange({ ...value, endDate: v })
          }}
        />
      </div>
      <label className="block text-xs font-medium text-gray-500">
        Description
        <textarea
          value={value.description}
          onChange={(e) => {
            onChange({ ...value, description: e.target.value })
          }}
          rows={2}
          className="mt-1 w-full rounded-md border border-gray-300 px-2.5 py-1.5 text-sm text-gray-900 focus:border-gray-500 focus:outline-none"
        />
      </label>
      <div className="flex gap-2">
        <button
          type="button"
          onClick={onSave}
          className="rounded-md bg-gray-900 px-2.5 py-1 text-xs font-medium text-white hover:bg-gray-800"
        >
          Save
        </button>
        <button
          type="button"
          onClick={onCancel}
          className="rounded-md border border-gray-300 px-2.5 py-1 text-xs font-medium text-gray-700 hover:bg-gray-50"
        >
          Cancel
        </button>
      </div>
    </div>
  )
}

// The right-side editable CV components panel — props-in/callback-out,
// entirely controlled by the caller's own `data` state. Every edit
// here only ever mutates that local, in-memory CvData, NEVER the
// persona's own real data (no PUT/POST/DELETE against Personas/
// Skills/Experience anywhere in this file) — the whole point of this
// panel, per your own "not the global ones" instruction. See
// plan/ai/career/cv-builder/step-01-overview-and-data-model.md
// (addendum) and step-04-frontend-cv-builder.md.
export interface CvComponentsSidebarProps {
  data: CvData
  onChange: (updated: CvData) => void
}

export function CvComponentsSidebar({ data, onChange }: CvComponentsSidebarProps) {
  const [newSkill, setNewSkill] = useState('')
  const [editingExperienceIndex, setEditingExperienceIndex] = useState<number | null>(null)
  const [experienceForm, setExperienceForm] = useState<CvExperienceEntry>(emptyExperience)

  const addSkill = () => {
    const skill = newSkill.trim()
    if (!skill) return
    onChange({ ...data, skills: [...data.skills, skill] })
    setNewSkill('')
  }

  const removeSkill = (index: number) => {
    onChange({ ...data, skills: data.skills.filter((_, i) => i !== index) })
  }

  const startAddExperience = () => {
    setEditingExperienceIndex(data.experience.length)
    setExperienceForm(emptyExperience)
  }

  const startEditExperience = (index: number) => {
    setEditingExperienceIndex(index)
    setExperienceForm(data.experience[index])
  }

  const cancelExperienceEdit = () => {
    setEditingExperienceIndex(null)
    setExperienceForm(emptyExperience)
  }

  const saveExperience = () => {
    if (editingExperienceIndex === null) return
    const next = [...data.experience]
    if (editingExperienceIndex < next.length) {
      next[editingExperienceIndex] = experienceForm
    } else {
      next.push(experienceForm)
    }
    onChange({ ...data, experience: next })
    cancelExperienceEdit()
  }

  const removeExperience = (index: number) => {
    onChange({ ...data, experience: data.experience.filter((_, i) => i !== index) })
    if (editingExperienceIndex === index) cancelExperienceEdit()
  }

  return (
    <div className="w-80 shrink-0 space-y-6 rounded-md border border-gray-200 p-4 shadow-sm">
      <div>
        <h3 className="mb-2 text-xs font-semibold uppercase tracking-wide text-gray-500">Personal Details</h3>
        <div className="space-y-2">
          <Field
            label="Headline"
            value={data.headline}
            onChange={(v) => {
              onChange({ ...data, headline: v })
            }}
          />
          <label className="block text-xs font-medium text-gray-500">
            Summary
            <textarea
              value={data.summary}
              onChange={(e) => {
                onChange({ ...data, summary: e.target.value })
              }}
              rows={3}
              className="mt-1 w-full rounded-md border border-gray-300 px-2.5 py-1.5 text-sm text-gray-900 focus:border-gray-500 focus:outline-none"
            />
          </label>
          <Field
            label="Location"
            value={data.location}
            onChange={(v) => {
              onChange({ ...data, location: v })
            }}
          />
        </div>
      </div>

      <div>
        <h3 className="mb-2 text-xs font-semibold uppercase tracking-wide text-gray-500">Skills</h3>
        <div className="mb-2 flex flex-wrap gap-1.5">
          {data.skills.map((skill, i) => (
            <span
              key={`${skill}-${i}`}
              className="inline-flex items-center gap-1 rounded-full bg-gray-100 px-2.5 py-1 text-xs text-gray-800"
            >
              {skill}
              <button
                type="button"
                onClick={() => {
                  removeSkill(i)
                }}
                className="text-gray-400 hover:text-gray-700"
                aria-label={`Remove ${skill}`}
              >
                <XIcon />
              </button>
            </span>
          ))}
          {data.skills.length === 0 && <p className="text-xs text-gray-400">No skills yet.</p>}
        </div>
        <div className="flex gap-1.5">
          <input
            type="text"
            value={newSkill}
            onChange={(e) => {
              setNewSkill(e.target.value)
            }}
            onKeyDown={(e) => {
              if (e.key === 'Enter') {
                e.preventDefault()
                addSkill()
              }
            }}
            placeholder="Add a skill…"
            className="w-full rounded-md border border-gray-300 px-2.5 py-1.5 text-sm text-gray-900 focus:border-gray-500 focus:outline-none"
          />
          <button
            type="button"
            onClick={addSkill}
            className="shrink-0 rounded-md bg-gray-900 px-2.5 py-1.5 text-white hover:bg-gray-800"
            aria-label="Add skill"
          >
            <PlusIcon />
          </button>
        </div>
      </div>

      <div>
        <h3 className="mb-2 text-xs font-semibold uppercase tracking-wide text-gray-500">Experience</h3>
        <div className="space-y-2">
          {data.experience.map((entry, i) =>
            editingExperienceIndex === i ? (
              <ExperienceForm
                key={i}
                value={experienceForm}
                onChange={setExperienceForm}
                onSave={saveExperience}
                onCancel={cancelExperienceEdit}
              />
            ) : (
              <div key={i} className="flex items-start justify-between gap-2 rounded-md border border-gray-200 p-2">
                <button
                  type="button"
                  onClick={() => {
                    startEditExperience(i)
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
                    removeExperience(i)
                  }}
                  className="shrink-0 text-gray-400 hover:text-red-600"
                  aria-label="Remove experience entry"
                >
                  <XIcon />
                </button>
              </div>
            ),
          )}
          {editingExperienceIndex === data.experience.length && (
            <ExperienceForm value={experienceForm} onChange={setExperienceForm} onSave={saveExperience} onCancel={cancelExperienceEdit} />
          )}
        </div>
        {editingExperienceIndex === null && (
          <button
            type="button"
            onClick={startAddExperience}
            className="mt-2 flex items-center gap-1 text-xs font-medium text-gray-700 hover:text-gray-900"
          >
            <PlusIcon /> Add experience
          </button>
        )}
      </div>
    </div>
  )
}
