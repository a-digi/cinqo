// Pure rules for reviewing a proposal before it is created (step 112): the drafts the user edits,
// what is accepted, what is wrong with it, how skills are added, and the duplicate hints.

import type { PersonaProposal, ProposedPersona } from '../api'
import { MAX_SKILLS, MAX_SHORT, checkAccepted } from './personaProposal'

export interface Draft {
  include: boolean
  persona: ProposedPersona
}

// A copy of every proposed persona to edit, all included.
export function draftsFrom(proposal: PersonaProposal): Draft[] {
  return proposal.personas.map((p) => ({ include: true, persona: structuredClone(p) }))
}

export function accepted(drafts: Draft[]): ProposedPersona[] {
  return drafts.filter((d) => d.include).map((d) => d.persona)
}

// What stops the accepted personas from being created, or null.
export function reviewProblem(drafts: Draft[]): string | null {
  return checkAccepted(accepted(drafts))
}

// The skills with text added: trimmed, blank and duplicates (any case) ignored, the cap kept.
export function addSkill(skills: string[], text: string): string[] {
  const t = text.trim()
  if (t === '' || t.length > MAX_SHORT || skills.length >= MAX_SKILLS) return skills
  if (skills.some((s) => s.toLowerCase() === t.toLowerCase())) return skills
  return [...skills, t]
}

export function removeAt<T>(list: T[], index: number): T[] {
  return list.filter((_, i) => i !== index)
}

// Says when a proposed persona looks like one the profile already has: by the AI's own hint (the
// id of an existing persona), or by the same name. Null when it does not.
export function duplicateHint(p: ProposedPersona, existing: { id: string; name: string }[]): string | null {
  if (p.possibleDuplicateOf !== null) {
    const hinted = existing.find((e) => e.id === p.possibleDuplicateOf)
    return hinted ? `Looks like your existing persona "${hinted.name}".` : 'The AI thinks this looks like a persona you already have.'
  }
  const same = existing.find((e) => e.name.trim().toLowerCase() === p.name.trim().toLowerCase())
  return same ? `You already have a persona called "${same.name}".` : null
}

// "Create 2 personas", "Create 1 persona", or what is missing.
export function createLabel(drafts: Draft[]): string {
  const n = drafts.filter((d) => d.include).length
  return n === 1 ? 'Create 1 persona' : `Create ${n} personas`
}
