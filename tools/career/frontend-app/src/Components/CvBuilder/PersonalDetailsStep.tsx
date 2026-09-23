import type { CvData } from '../../api'
import { Field } from '../Field/Field'

export interface CvStepProps {
  data: CvData
  onChange: (updated: CvData) => void
}

// Wizard step 2 — same fields, same "edits this local CvData only,
// never the persona's own real data" contract every CV Builder step
// shares. See plan/ai/career/cv-builder/step-04-frontend-cv-builder.md.
export function PersonalDetailsStep({ data, onChange }: CvStepProps) {
  return (
    <div className="space-y-3">
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
          rows={4}
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
  )
}
