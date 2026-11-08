// What the user already has (step 113): the skills, employers and job titles of the profile's
// existing personas, so a proposal can say which of its items are NOT in that data. It compares
// text only (trimmed, case-insensitive): a reworded skill counts as new. It is a hint for the
// review, never a verdict and never a block. Pure, so it can be script-checked.

import type { ProposedPersona } from '../api'

export interface KnownData {
  skills: Set<string>
  companies: Set<string>
  titles: Set<string>
}

const norm = (s: string) => s.trim().toLowerCase()

// The data of the existing personas (what fetchPersonaDetails returns for each), as sets.
export function buildKnown(personas: { skills: string[]; experience: { company: string; title: string }[] }[]): KnownData {
  const known: KnownData = { skills: new Set(), companies: new Set(), titles: new Set() }
  for (const p of personas) {
    for (const s of p.skills) known.skills.add(norm(s))
    for (const r of p.experience) {
      if (norm(r.company) !== '') known.companies.add(norm(r.company))
      if (norm(r.title) !== '') known.titles.add(norm(r.title))
    }
  }
  return known
}

// With nothing to compare against (no skill and no experience anywhere) every item would be marked,
// which says nothing: then nothing is marked.
export function hasBasis(k: KnownData): boolean {
  return k.skills.size > 0 || k.companies.size > 0 || k.titles.size > 0
}

export const isNewSkill = (k: KnownData, skill: string) => norm(skill) !== '' && !k.skills.has(norm(skill))
export const isNewCompany = (k: KnownData, company: string) => norm(company) !== '' && !k.companies.has(norm(company))
export const isNewTitle = (k: KnownData, title: string) => norm(title) !== '' && !k.titles.has(norm(title))

export interface NewItems {
  skills: string[]
  companies: string[]
  titles: string[]
}

function unique(list: string[]): string[] {
  const seen = new Set<string>()
  const out: string[] = []
  for (const raw of list) {
    const t = raw.trim()
    if (t !== '' && !seen.has(norm(t))) {
      seen.add(norm(t))
      out.push(t)
    }
  }
  return out
}

// The items of a proposed persona that are not in the user's data, each once, in order.
export function newItems(k: KnownData, p: ProposedPersona): NewItems {
  return {
    skills: unique(p.skills.filter((s) => isNewSkill(k, s))),
    companies: unique(p.experience.map((r) => r.company).filter((c) => isNewCompany(k, c))),
    titles: unique(p.experience.map((r) => r.title).filter((t) => isNewTitle(k, t))),
  }
}

export function countNew(items: NewItems): number {
  return items.skills.length + items.companies.length + items.titles.length
}

// "Not in your data: skills Docker, Kubernetes; employers Acme; job titles Lead", or null when
// every item is in the user's data.
export function newItemsText(items: NewItems): string | null {
  const parts: string[] = []
  if (items.skills.length > 0) parts.push(`${items.skills.length === 1 ? 'skill' : 'skills'} ${items.skills.join(', ')}`)
  if (items.companies.length > 0) parts.push(`${items.companies.length === 1 ? 'employer' : 'employers'} ${items.companies.join(', ')}`)
  if (items.titles.length > 0) parts.push(`${items.titles.length === 1 ? 'job title' : 'job titles'} ${items.titles.join(', ')}`)
  return parts.length === 0 ? null : `Not in your data: ${parts.join('; ')}.`
}
