import { PersonaSwitcher } from '../Persona/PersonaSwitcher/PersonaSwitcher'
import type { CvTemplate } from '../../api'

export interface PersonaAndTemplateStepProps {
  personaId: string | null
  onPersonaChange: (id: string) => void
  templates: CvTemplate[]
  templateId: string
  onTemplateChange: (id: string) => void
}

// Wizard step 1 — pick who this CV is for and which design to render
// it with. Changing the persona always re-fetches that persona's own
// fresh defaults (CvBuilderPage's own onPersonaChange), discarding
// whatever was in the later steps — deliberately simple, matching the
// single-page builder's own prior behavior exactly. See
// plan/ai/career/cv-builder/step-04-frontend-cv-builder.md.
export function PersonaAndTemplateStep({
  personaId,
  onPersonaChange,
  templates,
  templateId,
  onTemplateChange,
}: PersonaAndTemplateStepProps) {
  return (
    <div className="space-y-4">
      <PersonaSwitcher personaId={personaId} onChange={onPersonaChange} />

      <div>
        <h3 className="mb-2 text-xs font-semibold uppercase tracking-wide text-gray-500">Template</h3>
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
          {templates.map((t) => (
            <button
              key={t.id}
              type="button"
              onClick={() => {
                onTemplateChange(t.id)
              }}
              className={`rounded-md border p-3 text-left text-sm shadow-sm ${
                templateId === t.id ? 'border-gray-900 ring-1 ring-gray-900' : 'border-gray-200 hover:border-gray-300'
              }`}
            >
              <p className="font-medium text-gray-900">{t.name}</p>
              <p className="mt-1 text-xs text-gray-500">{t.description}</p>
            </button>
          ))}
        </div>
      </div>
    </div>
  )
}
