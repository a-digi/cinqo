import type { CvData } from '../../api'
import { Field } from '../Field/Field'

export interface TitleAndReviewStepProps {
  title: string
  onTitleChange: (v: string) => void
  data: CvData
  templateName: string
}

// Wizard step 5 (final) — name it, glance over what's about to be
// generated, then Generate (CvBuilderPage's own button, outside this
// component — this step is display/input only, no submit action of
// its own).
export function TitleAndReviewStep({ title, onTitleChange, data, templateName }: TitleAndReviewStepProps) {
  return (
    <div className="space-y-4">
      <Field label="Title" value={title} onChange={onTitleChange} />

      <div className="rounded-md border border-gray-200 bg-gray-50 p-3 text-sm text-gray-700">
        <p>
          <span className="font-medium text-gray-900">{data.fullName || 'Untitled'}</span> — {templateName}
        </p>
        {data.headline && <p className="mt-1 text-gray-600">{data.headline}</p>}
        <p className="mt-2 text-xs text-gray-500">
          {data.skills.length} skill{data.skills.length === 1 ? '' : 's'} · {data.experience.length} experience entr
          {data.experience.length === 1 ? 'y' : 'ies'}
        </p>
      </div>
    </div>
  )
}
