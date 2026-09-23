import type { CvExperienceEntry } from '../../api'
import { Field } from '../Field/Field'

// The add/edit form for one experience entry — shared by BOTH the
// "editing an existing entry" and "adding a new one" cases in
// ExperienceStep.tsx. Extracted to its own file (previously inlined
// twice, once per case — a real bug, caught by live testing: the
// "adding new" case's own copy was accidentally never wired into the
// render tree at all, since it was written to only ever render inside
// a .map() over EXISTING entries). See
// plan/ai/career/cv-builder/step-04-frontend-cv-builder.md.
export interface ExperienceFormProps {
  value: CvExperienceEntry
  onChange: (updated: CvExperienceEntry) => void
  onSave: () => void
  onCancel: () => void
}

export function ExperienceForm({ value, onChange, onSave, onCancel }: ExperienceFormProps) {
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
