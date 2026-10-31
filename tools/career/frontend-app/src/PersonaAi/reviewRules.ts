// Pure rules for the AI's suggested personas (steps 112 and 113): which suggestions still wait for
// the user, the persona exactly as it will be saved, the decisions to send (one persona at a time:
// Accept creates it at once, Decline creates nothing), how skills are added, and the duplicate
// hints.

import type { PersonaAiRun, PersonaDecision, ProposedPersona } from '../api'
import { MAX_SKILLS, MAX_SHORT } from './personaProposal'

export interface Suggestion {
  run: PersonaAiRun
  index: number
  persona: ProposedPersona
}

// The suggestions still waiting for a decision: from every ready, open run, the proposed personas
// whose index is not decided yet, newest run first (the order the runs come in).
export function waitingSuggestions(runs: PersonaAiRun[]): Suggestion[] {
  const out: Suggestion[] = []
  for (const run of runs) {
    if (run.status !== 'ready' || run.decidedAt !== undefined) continue
    run.proposal?.personas.forEach((persona, index) => {
      if (!run.decided.includes(index)) out.push({ run, index, persona })
    })
  }
  return out
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

export function acceptDecision(index: number, persona: ProposedPersona): PersonaDecision {
  return { index, decision: 'accepted', persona: finalPersona(persona) }
}

export function declineDecision(index: number): PersonaDecision {
  return { index, decision: 'declined' }
}

// The salary as shown in the preview: "Not set" for 0 (nothing was given).
export function salaryText(minSalary: number): string {
  return minSalary > 0 ? minSalary.toLocaleString('en-US') : 'Not set'
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
