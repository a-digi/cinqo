import type { JobRequirement, JobRequirementInput } from '../../api'

// Pure rules of the Requirements card, checked before a list is sent (the backend checks the same limits again).

export const MAX_REQUIREMENTS = 30
export const MAX_REQUIREMENT_LENGTH = 120

// The tiers, most important first.
export const TIERS = [
  { value: 3, label: 'Must-have' },
  { value: 2, label: 'Important' },
  { value: 1, label: 'Nice to have' },
] as const

// Why a term cannot be added, or '' when it can: blank, too long, the list is full, or it is already there.
export function requirementProblem(list: readonly { term: string }[], term: string): string {
  const t = term.trim()
  if (t === '') return 'Write a requirement.'
  if (Array.from(t).length > MAX_REQUIREMENT_LENGTH) return `A requirement can have at most ${String(MAX_REQUIREMENT_LENGTH)} characters.`
  if (list.length >= MAX_REQUIREMENTS) return `A job can have at most ${String(MAX_REQUIREMENTS)} requirements.`
  if (list.some((r) => r.term.toLowerCase() === t.toLowerCase())) return `"${t}" is already listed.`
  return ''
}

// What is sent to the server: the list as it stands.
export function toInputs(list: readonly JobRequirement[]): JobRequirementInput[] {
  return list.map((r) => ({ term: r.term, tier: r.tier }))
}

export function addedTo(list: readonly JobRequirement[], term: string, tier: number): JobRequirementInput[] {
  return [...toInputs(list), { term: term.trim(), tier }]
}

export function withTier(list: readonly JobRequirement[], id: string, tier: number): JobRequirementInput[] {
  return list.map((r) => ({ term: r.term, tier: r.id === id ? tier : r.tier }))
}

export function without(list: readonly JobRequirement[], id: string): JobRequirementInput[] {
  return list.filter((r) => r.id !== id).map((r) => ({ term: r.term, tier: r.tier }))
}

// The list grouped by tier, most important first; empty tiers left out. Order inside a tier is the saved order.
export function groupedByTier(list: readonly JobRequirement[]): { tier: number; label: string; items: JobRequirement[] }[] {
  return TIERS.map((t) => ({ tier: t.value, label: t.label, items: list.filter((r) => r.tier === t.value) })).filter(
    (g) => g.items.length > 0,
  )
}

// The suggestions that are not already a requirement (ignoring case).
export function freshSuggestions(suggestions: readonly string[], list: readonly { term: string }[]): string[] {
  const have = new Set(list.map((r) => r.term.toLowerCase()))
  return suggestions.filter((s) => !have.has(s.toLowerCase()))
}
