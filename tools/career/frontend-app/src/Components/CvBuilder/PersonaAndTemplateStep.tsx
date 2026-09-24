import { useMemo, useState } from 'react'
import { PersonaSwitcher } from '../Persona/PersonaSwitcher/PersonaSwitcher'
import { Dropdown } from '../Dropdown/Dropdown'
import { EyeIcon } from '../../Shared/Icons/icons'
import { previewImages } from './previewImages'
import type { CvTemplate } from '../../api'

export interface PersonaAndTemplateStepProps {
  personaId: string | null
  onPersonaChange: (id: string) => void
  templates: CvTemplate[]
  templateId: string
  onTemplateChange: (id: string) => void
  hasProfilePhoto: boolean
  attachPhoto: boolean
  onAttachPhotoChange: (attach: boolean) => void
  canPreview: boolean
  previewing: boolean
  onPreviewTemplate: (templateId: string) => void
}

const ALL_CATEGORIES = 'All'

// Wizard step 1 — pick who this CV is for and which design to render
// it with. Changing the persona always re-fetches that persona's own
// fresh defaults (CvBuilderPage's own onPersonaChange), discarding
// whatever was in the later steps — deliberately simple, matching the
// single-page builder's own prior behavior exactly. See
// plan/ai/career/cv-builder/step-04-frontend-cv-builder.md.
//
// With 26 templates, a flat grid stopped being browsable — this now
// mirrors a template-gallery layout (style pills + a profession
// filter) so a user can narrow down to "Modern" or "best for Software
// & IT" instead of scrolling past everything. Category/profession
// options are derived from whatever the backend's own metas[] actually
// contains (never hardcoded here), so a new template just shows up in
// the right filter automatically. See
// plan/ai/career/cv-builder/step-06-template-gallery-and-filters.md.
export function PersonaAndTemplateStep({
  personaId,
  onPersonaChange,
  templates,
  templateId,
  onTemplateChange,
  hasProfilePhoto,
  attachPhoto,
  onAttachPhotoChange,
  canPreview,
  previewing,
  onPreviewTemplate,
}: PersonaAndTemplateStepProps) {
  const [category, setCategory] = useState<string>(ALL_CATEGORIES)
  const [professions, setProfessions] = useState<string[]>([])

  const categories = useMemo(() => {
    const seen = new Set<string>()
    const ordered: string[] = []
    for (const t of templates) {
      if (!seen.has(t.category)) {
        seen.add(t.category)
        ordered.push(t.category)
      }
    }
    return [ALL_CATEGORIES, ...ordered]
  }, [templates])

  const professionOptions = useMemo(() => {
    const all = new Set<string>()
    for (const t of templates) {
      for (const p of t.bestFor) all.add(p)
    }
    return [...all].sort().map((p) => ({ value: p, label: p }))
  }, [templates])

  const filteredTemplates = templates.filter((t) => {
    if (category !== ALL_CATEGORIES && t.category !== category) return false
    if (professions.length > 0 && !professions.some((p) => t.bestFor.includes(p))) return false
    return true
  })

  const filtersActive = category !== ALL_CATEGORIES || professions.length > 0

  return (
    <div className="space-y-4">
      <PersonaSwitcher personaId={personaId} onChange={onPersonaChange} />

      {hasProfilePhoto && (
        <label className="flex items-center gap-2 text-sm text-gray-700">
          <input
            type="checkbox"
            checked={attachPhoto}
            onChange={(e) => {
              onAttachPhotoChange(e.target.checked)
            }}
            className="h-4 w-4 rounded border-gray-300 text-gray-900 focus:ring-gray-900"
          />
          Include this profile's photo in the CV
        </label>
      )}

      <div>
        <h3 className="mb-2 text-xs font-semibold uppercase tracking-wide text-gray-500">Template</h3>

        <div className="mb-3 flex flex-wrap items-end gap-3">
          <div className="flex flex-wrap gap-1.5">
            {categories.map((c) => (
              <button
                key={c}
                type="button"
                onClick={() => {
                  setCategory(c)
                }}
                className={`rounded-full border px-3 py-1 text-xs font-medium ${
                  category === c ? 'border-gray-900 bg-gray-900 text-white' : 'border-gray-200 text-gray-600 hover:border-gray-300'
                }`}
              >
                {c}
              </button>
            ))}
          </div>

          <Dropdown
            multiple
            label="Best for"
            placeholder="Any profession"
            searchable
            options={professionOptions}
            value={professions}
            onChange={setProfessions}
          />

          {filtersActive && (
            <button
              type="button"
              onClick={() => {
                setCategory(ALL_CATEGORIES)
                setProfessions([])
              }}
              className="pb-1 text-xs text-gray-500 underline hover:text-gray-700"
            >
              Clear filters
            </button>
          )}
        </div>

        {filteredTemplates.length === 0 && <p className="text-sm text-gray-400">No templates match these filters.</p>}

        <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 md:grid-cols-4">
          {filteredTemplates.map((t) => (
            <div
              key={t.id}
              className={`flex h-full flex-col overflow-hidden rounded-md border text-sm shadow-sm ${
                templateId === t.id ? 'border-gray-900 ring-1 ring-gray-900' : 'border-gray-200 hover:border-gray-300'
              }`}
            >
              <button
                type="button"
                onClick={() => {
                  onTemplateChange(t.id)
                }}
                className="block w-full flex-1 text-left"
              >
                <div className="aspect-[600/780] w-full bg-gray-50">
                  {previewImages[t.id] && (
                    <img src={previewImages[t.id]} alt={`${t.name} preview`} className="h-full w-full object-cover" />
                  )}
                </div>
                <div className="p-2.5">
                  <p className="font-medium text-gray-900">{t.name}</p>
                  <p className="mt-1 text-xs text-gray-500">{t.description}</p>
                  <p className="mt-1.5 text-[11px] text-gray-400">{t.bestFor.join(' · ')}</p>
                </div>
              </button>
              <button
                type="button"
                disabled={!canPreview || previewing}
                onClick={() => {
                  onPreviewTemplate(t.id)
                }}
                className="flex w-full shrink-0 items-center justify-center gap-1.5 border-t border-gray-200 bg-gray-900 py-1.5 text-xs font-medium text-white hover:bg-gray-800 disabled:opacity-50"
              >
                <EyeIcon />
                {previewing ? 'Preparing preview…' : 'Preview'}
              </button>
            </div>
          ))}
        </div>
      </div>
    </div>
  )
}
