import type { Persona } from '../../api'
import { PersonaPicker } from './PersonaPicker'

// Wizard step 1: who this CV is for, picked from the list of personas. Picking one loads that persona's own fresh
// defaults for the later steps (CvBuilderPage's own persona handling), discarding whatever was in them. The template
// is the next step. See plan/ai/tools/career/step-123-cv-builder-persona-list.md.
export function PersonaStep({ personaId, onPick }: { personaId: string | null; onPick: (persona: Persona) => void }) {
  return (
    <div className="space-y-4">
      <PersonaPicker personaId={personaId} onSelect={onPick} />
    </div>
  )
}
