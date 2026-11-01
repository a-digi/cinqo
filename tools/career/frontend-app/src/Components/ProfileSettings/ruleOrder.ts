// The pure rules of the order of a profile's auto-add rules on the settings page (step 143): moving a rule and finding the
// rules that can never apply. A rule fits a job when the job's match is at least the rule's percentage; the first fitting
// rule of the list decides, so the order of the list is what matters. Kept apart from the page so it can be checked alone.
import type { ProfileMatchRule } from '../../api'

// The list after the rule at `from` was moved to place `to` (both positions in the list, 0 first). The same list when the
// move changes nothing or a place is not in the list. Never changes the list it is given.
export function moveRule(rules: ProfileMatchRule[], from: number, to: number): ProfileMatchRule[] {
  const size = rules.length
  if (!Number.isInteger(from) || !Number.isInteger(to) || from < 0 || to < 0 || from >= size || to >= size || from === to) return rules
  const next = rules.slice()
  const [moved] = next.splice(from, 1)
  next.splice(to, 0, moved)
  return next
}

// The ids of the rules in list order, what the order route takes.
export function idsInOrder(rules: ProfileMatchRule[]): string[] {
  return rules.map((r) => r.id)
}

// Whether two lists hold the same rules in the same order.
export function sameOrder(a: ProfileMatchRule[], b: ProfileMatchRule[]): boolean {
  return a.length === b.length && a.every((rule, i) => rule.id === b[i].id)
}

// The earlier rule that makes a rule useless: its place in the list (1 first, as the page numbers the rules) and its percentage.
export interface Blocker {
  place: number
  minPercent: number
}

// The rules that can never apply, by id, each with the earlier rule that always takes first what it would take: a rule
// fits every job whose match reaches its percentage, so when an earlier rule has an equal or lower percentage, every job
// that fits this rule fits the earlier one too and the earlier one wins. The earlier rule named is the first such one.
export function neverApplies(rules: ProfileMatchRule[]): ReadonlyMap<string, Blocker> {
  const out = new Map<string, Blocker>()
  rules.forEach((rule, i) => {
    const at = rules.slice(0, i).findIndex((earlier) => earlier.minPercent <= rule.minPercent)
    if (at >= 0) out.set(rule.id, { place: at + 1, minPercent: rules[at].minPercent })
  })
  return out
}
