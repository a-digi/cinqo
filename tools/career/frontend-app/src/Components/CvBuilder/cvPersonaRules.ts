// Pure rules for the personas a CV is attached to (step 114). No React, so they can be script-checked.
import type { CvDocument, Persona } from '../../api'

// The personas the CV can still be attached to: those of the profile of the persona it was built
// from (a CV holds one person's data) that it is not attached to yet, by name. Empty while the
// built-from persona is not in the list.
export function attachablePersonas(doc: Pick<CvDocument, 'personas'>, all: Persona[]): Persona[] {
  const builtFrom = doc.personas.find((p) => p.builtFrom)
  const profileId = all.find((p) => p.id === builtFrom?.id)?.profileId
  if (profileId === undefined) return []
  const linked = new Set(doc.personas.map((p) => p.id))
  return all.filter((p) => p.profileId === profileId && !linked.has(p.id)).sort((a, b) => a.name.localeCompare(b.name))
}

// "Backend, Data Lead": every persona of the CV, built-from first (the order the server gives).
export function personaNames(doc: Pick<CvDocument, 'personas' | 'personaName'>): string {
  if (doc.personas.length > 0) return doc.personas.map((p) => p.name).join(', ')
  return doc.personaName ?? ''
}

// The CVs linked to a persona (built from it or attached to it), newest first, each with whether
// the persona is the one it was built from.
export function cvsOfPersona<T extends Pick<CvDocument, 'personas' | 'createdAt'>>(
  docs: T[],
  personaId: string,
): { doc: T; builtFrom: boolean }[] {
  return docs
    .flatMap((doc) => {
      const link = doc.personas.find((p) => p.id === personaId)
      return link ? [{ doc, builtFrom: link.builtFrom }] : []
    })
    .sort((a, b) => (a.doc.createdAt < b.doc.createdAt ? 1 : a.doc.createdAt > b.doc.createdAt ? -1 : 0))
}
