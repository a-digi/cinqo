// resolveDuplicateLabels.ts — the frontend independently calls
// Career's own existing list_personas/get_persona_details HTTP routes
// (not through the AI — this is the app's own UI fetching its own
// data the normal way) to resolve a proposal's possibleDuplicateOf IDs
// into human-readable labels for display. Skills' own
// possibleDuplicateOf is already a readable string (no ID exists for a
// skill — see the plan's own asymmetry note) and needs no resolution.
// See plan/ai/tools/career/import-cv/step-06-proposal-parsing-and-review-ui.md.
import { fetchPersonas, fetchPersonaDetails, type Persona } from '../../api'

export interface DuplicateLabels {
  personas: Record<string, string>
  experience: Record<string, string>
  // The raw list itself, not just id->label — step 7's own "select an
  // existing persona" dropdown (shown when the persona row is
  // unchecked) is built from this, avoiding a second, redundant fetch.
  personaList: Persona[]
}

export async function resolveDuplicateLabels(): Promise<DuplicateLabels> {
  const personas = await fetchPersonas()
  const personaLabels: Record<string, string> = {}
  const experienceLabels: Record<string, string> = {}

  for (const persona of personas) {
    personaLabels[persona.id] = persona.name
    const details = await fetchPersonaDetails(persona.id)
    for (const exp of details.experience) {
      experienceLabels[exp.id] = `${persona.name}: ${exp.company} (${exp.title})`
    }
  }

  return { personas: personaLabels, experience: experienceLabels, personaList: personas }
}
