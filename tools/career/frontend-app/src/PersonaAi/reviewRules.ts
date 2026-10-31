// Pure rules for deciding on a proposal (steps 112 and 113): the drafts the user edits, who is
// accepted, declined or still undecided, what stops saving, how skills are added, and the
// duplicate hints. Nothing is pre-accepted: a persona is created only after the user pressed
// Accept on it.

import type { PersonaDecision, PersonaProposal, ProposedPersona } from '../api'
import { MAX_SKILLS, MAX_SHORT, checkAccepted } from './personaProposal'

export type DraftDecision = 'undecided' | 'accepted' | 'declined'

export interface Draft {
  decision: DraftDecision
  persona: ProposedPersona
}

// A copy of every proposed persona to edit, none of them decided.
export function draftsFrom(proposal: PersonaProposal): Draft[] {
  return proposal.personas.map((p) => ({ decision: 'undecided', persona: structuredClone(p) }))
}

// The drafts with the persona at index decided (pressing the same decision again undoes it).
export function decide(drafts: Draft[], index: number, decision: 'accepted' | 'declined'): Draft[] {
  return drafts.map((d, i) => (i === index ? { ...d, decision: d.decision === decision ? 'undecided' : decision } : d))
}

// Every persona declined at once.
export function declineAll(drafts: Draft[]): Draft[] {
  return drafts.map((d) => ({ ...d, decision: 'declined' }))
}

export interface Counts {
  accepted: number
  declined: number
  undecided: number
}

export function counts(drafts: Draft[]): Counts {
  const c: Counts = { accepted: 0, declined: 0, undecided: 0 }
  for (const d of drafts) c[d.decision]++
  return c
}

export function countsLabel(drafts: Draft[]): string {
  const c = counts(drafts)
  return `${c.accepted} accepted, ${c.declined} declined, ${c.undecided} undecided`
}

export function accepted(drafts: Draft[]): ProposedPersona[] {
  return drafts.filter((d) => d.decision === 'accepted').map((d) => d.persona)
}

// What stops the decisions from being saved, or null: every persona needs a decision, and what was
// accepted must be valid. Declining everything is fine.
export function reviewProblem(drafts: Draft[]): string | null {
  const open = counts(drafts).undecided
  if (open > 0)
    return open === 1
      ? 'One persona still needs a decision: accept or decline it.'
      : `${open} personas still need a decision: accept or decline each.`
  const keep = accepted(drafts)
  return keep.length === 0 ? null : checkAccepted(keep)
}

// A persona exactly as it will be saved: every text trimmed and the skills trimmed and
// de-duplicated (the first spelling stays, any case). The server does the same, so the preview shows
// what the saved persona will contain. The input is not changed.
export function finalPersona(p: ProposedPersona): ProposedPersona {
  const seen = new Set<string>()
  const skills: string[] = []
  for (const raw of p.skills) {
    const s = raw.trim()
    if (s === '' || seen.has(s.toLowerCase())) continue
    seen.add(s.toLowerCase())
    skills.push(s)
  }
  return {
    ...p,
    name: p.name.trim(),
    description: p.description.trim(),
    details: {
      headline: p.details.headline.trim(),
      summary: p.details.summary.trim(),
      location: p.details.location.trim(),
      desiredTitles: p.details.desiredTitles.trim(),
      desiredLocations: p.details.desiredLocations.trim(),
      minSalary: p.details.minSalary,
    },
    skills,
    experience: p.experience.map((r) => ({
      company: r.company.trim(),
      title: r.title.trim(),
      startDate: r.startDate.trim(),
      endDate: r.endDate.trim(),
      description: r.description.trim(),
    })),
  }
}

// The decisions to send, one for every proposed persona (accepted ones with their content as it
// will be saved); null while something is undecided.
export function toDecisions(drafts: Draft[]): PersonaDecision[] | null {
  if (counts(drafts).undecided > 0) return null
  return drafts.map((d, index) =>
    d.decision === 'accepted' ? { index, decision: 'accepted', persona: finalPersona(d.persona) } : { index, decision: 'declined' },
  )
}

// The text of the button that leaves the decisions for the preview.
export function continueLabel(drafts: Draft[]): string {
  const c = counts(drafts)
  return c.undecided === 0 && c.accepted === 0 ? 'Continue: decline all' : 'Continue to preview'
}

// What the preview says is about to happen, in one line.
export function previewSummary(drafts: Draft[]): string {
  const c = counts(drafts)
  if (c.accepted === 0)
    return `Nothing will be created: ${c.declined === 1 ? 'the persona is' : `all ${c.declined} personas are`} declined.`
  const made = c.accepted === 1 ? '1 persona will be created' : `${c.accepted} personas will be created`
  return c.declined === 0 ? `${made}.` : `${made}, ${c.declined} declined.`
}

// The names of the declined personas, in proposal order, for the preview.
export function declinedNames(drafts: Draft[]): string[] {
  return drafts.filter((d) => d.decision === 'declined').map((d) => d.persona.name.trim() || '(no name)')
}

// The salary as shown in the preview: "Not set" for 0 (nothing was given).
export function salaryText(minSalary: number): string {
  return minSalary > 0 ? minSalary.toLocaleString('en-US') : 'Not set'
}

// The save button's text: how many will be created, or that everything is declined.
export function saveLabel(drafts: Draft[]): string {
  const c = counts(drafts)
  if (c.accepted === 0 && c.undecided === 0) return 'Decline all and close'
  return c.accepted === 1 ? 'Save 1 persona' : `Save ${c.accepted} personas`
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
