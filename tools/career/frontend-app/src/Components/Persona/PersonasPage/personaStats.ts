// Pure numbers for the Personas page's overview. No React, so they can be script-checked.
export interface PersonaLike {
  description?: string
}

export interface PersonaSummary {
  personas: number
  // A description that is only whitespace counts as none.
  withDescription: number
  withoutDescription: number
}

export function summarizePersonas(personas: PersonaLike[]): PersonaSummary {
  const s: PersonaSummary = { personas: personas.length, withDescription: 0, withoutDescription: 0 }
  for (const p of personas) {
    if ((p.description ?? '').trim() !== '') s.withDescription++
    else s.withoutDescription++
  }
  return s
}
