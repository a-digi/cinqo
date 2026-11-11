// optimizeRules.ts — the pure rules of "Optimize with AI" (plan 258): what the button shows, how the suggestions are grouped, the word diff of a before and an
// after text, and what "approve all" and "decline all" send. No React and no fetching, so they can be tested alone.
import type { OptimizeDecided, OptimizeDecision, OptimizeRun, OptimizeSuggestion } from '../api'

// The title of a run's hidden conversation: the same for every driver, so the history reads alike.
export function optimizeConversationTitle(subjectName: string): string {
  return `Optimize with AI${subjectName ? `: ${subjectName}` : ''}`
}

export const NOTHING_TO_IMPROVE = 'The AI did not find anything to improve in these texts.'

// The groups of the review, in this order, and the kinds each holds.
const GROUPS: { heading: string; kinds: string[] }[] = [
  { heading: 'Summary', kinds: ['profileDetails', 'personaDetails'] },
  { heading: 'Experience', kinds: ['experience', 'customExperience'] },
  { heading: 'Education', kinds: ['education'] },
  { heading: 'Courses', kinds: ['course'] },
  { heading: 'Hobbies', kinds: ['hobby'] },
]

export interface SuggestionGroup {
  heading: string
  items: OptimizeSuggestion[]
}

// The suggestions by group, in the fixed order of the groups; inside a group they keep the order the AI handed them over. A kind this page does not know is shown last.
export function groupSuggestions(list: OptimizeSuggestion[]): SuggestionGroup[] {
  const groups: SuggestionGroup[] = GROUPS.map((g) => ({ heading: g.heading, items: list.filter((s) => g.kinds.includes(s.kind)) }))
  const known = new Set(GROUPS.flatMap((g) => g.kinds))
  groups.push({ heading: 'Other', items: list.filter((s) => !known.has(s.kind)) })
  return groups.filter((g) => g.items.length > 0)
}

export type OptimizeView =
  | { view: 'idle' }
  | { view: 'working'; run: OptimizeRun }
  | { view: 'review'; run: OptimizeRun; count: number; staleCount: number }
  | { view: 'failed'; run: OptimizeRun }

// What the button shows for a subject's runs (newest first): a run that is working, else one with suggestions to review, else the newest failed one, else nothing.
export function optimizeView(runs: OptimizeRun[]): OptimizeView {
  const working = runs.find((r) => r.status === 'generating')
  if (working) return { view: 'working', run: working }
  const ready = runs.find((r) => r.status === 'ready' && r.suggestions.length > 0)
  if (ready)
    return { view: 'review', run: ready, count: ready.suggestions.length, staleCount: ready.suggestions.filter((s) => s.stale).length }
  const failed = runs.find((r) => r.status === 'failed')
  if (failed) return { view: 'failed', run: failed }
  return { view: 'idle' }
}

export function reviewLabel(count: number): string {
  return `Review suggestions (${String(count)})`
}

// "Approve all" and "decline all": approving skips a stale suggestion (its text changed after the AI read it: it can only be declined); declining takes every one.
export function decisionsForAll(suggestions: OptimizeSuggestion[], approve: boolean): OptimizeDecision[] {
  return suggestions.filter((s) => !approve || !s.stale).map((s) => ({ suggestionId: s.id, approve }))
}

// The error of each refused decision, by suggestion id.
export function refusedOf(decided: OptimizeDecided): Record<string, string> {
  const out: Record<string, string> = {}
  for (const o of decided.outcomes) {
    if (!o.ok) out[o.suggestionId] = o.error ?? 'This could not be done.'
  }
  return out
}

// How many decisions in a batch were carried out as approvals: when it is more than none, what the page shows has changed.
export function approvedCount(decisions: OptimizeDecision[], decided: OptimizeDecided): number {
  const ok = new Set(decided.outcomes.filter((o) => o.ok).map((o) => o.suggestionId))
  return decisions.filter((d) => d.approve && ok.has(d.suggestionId)).length
}

export interface DiffPart {
  text: string
  changed: boolean
}

// The two texts as parts, the words that are not in the other text marked: `before` has the removed ones, `after` the added ones. Words are compared whole, white space
// is kept as it is. A pair of texts too long to compare word by word is shown as wholly changed.
export function diffWords(before: string, after: string): { before: DiffPart[]; after: DiffPart[] } {
  const a = before.split(/(\s+)/).filter((t) => t !== '')
  const b = after.split(/(\s+)/).filter((t) => t !== '')
  if (a.length * b.length > 4_000_000) {
    return { before: [{ text: before, changed: before !== '' }], after: [{ text: after, changed: after !== '' }] }
  }
  // The longest common subsequence of the tokens.
  const w = b.length + 1
  const lcs = new Uint32Array((a.length + 1) * w)
  for (let i = a.length - 1; i >= 0; i--) {
    for (let j = b.length - 1; j >= 0; j--) {
      lcs[i * w + j] = a[i] === b[j] ? lcs[(i + 1) * w + j + 1] + 1 : Math.max(lcs[(i + 1) * w + j], lcs[i * w + j + 1])
    }
  }
  const outA: DiffPart[] = []
  const outB: DiffPart[] = []
  const push = (out: DiffPart[], text: string, changed: boolean) => {
    const last = out.at(-1)
    if (last?.changed === changed) last.text += text
    else out.push({ text, changed })
  }
  let i = 0
  let j = 0
  while (i < a.length && j < b.length) {
    if (a[i] === b[j]) {
      push(outA, a[i], false)
      push(outB, b[j], false)
      i++
      j++
    } else if (lcs[(i + 1) * w + j] >= lcs[i * w + j + 1]) {
      push(outA, a[i], true)
      i++
    } else {
      push(outB, b[j], true)
      j++
    }
  }
  for (; i < a.length; i++) push(outA, a[i], true)
  for (; j < b.length; j++) push(outB, b[j], true)
  return { before: joinChanged(outA), after: joinChanged(outB) }
}

// Two changed parts with only white space between them are one changed phrase, shown as one mark.
function joinChanged(parts: DiffPart[]): DiffPart[] {
  const out: DiffPart[] = []
  parts.forEach((p, i) => {
    const between = !p.changed && p.text.trim() === '' && i > 0 && parts[i - 1].changed && i < parts.length - 1 && parts[i + 1].changed
    const part = between ? { text: p.text, changed: true } : p
    const last = out.at(-1)
    if (last?.changed === part.changed) last.text += part.text
    else out.push({ ...part })
  })
  return out
}

// A white-space-only part is not worth a mark: a changed run of spaces between two unchanged words is shown plain.
export function plainWhiteSpace(parts: DiffPart[]): DiffPart[] {
  return parts.map((p) => (p.changed && p.text.trim() === '' ? { ...p, changed: false } : p))
}
