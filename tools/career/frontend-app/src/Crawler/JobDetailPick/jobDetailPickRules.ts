// jobDetailPickRules.ts (plan 186, step 6) — the pure rules of the picking screen: what a pick is, how picks become the choices the backend
// builds the instructions from, which suggestion is taken up front, and how the page's text elements are filtered. No React, no calls.
import type {
  Choices,
  DescriptionHelp,
  DescriptionRule,
  FieldChoice,
  PageElement,
  PickCandidates,
  PickField,
  Ranked,
  RulePart,
} from './jobDetailPick'

// The four fields a person picks one by one; the description has its own row.
export const DATA_FIELDS: readonly PickField[] = ['title', 'company', 'location', 'postedAt']

export const FIELD_NAMES: Record<PickField, string> = {
  title: 'Title',
  company: 'Company',
  location: 'Location',
  postedAt: 'Posted date',
  description: 'Description',
}

// What a person chose for one field: where on the page it is read from.
export interface Pick {
  selector: string
  attribute: string
  // What the page showed there, for display only.
  value: string
  // 'suggestion' (one of the ranked candidates), 'element' (picked from the list of text elements) or 'stored' (read back from the
  // instructions the link already has).
  from: 'suggestion' | 'element' | 'stored'
}

// A pick per field; null is "not on this page" (the field is left out of the instructions).
export type Picks = Record<PickField, Pick | null>

export const NO_PICKS: Picks = { title: null, company: null, location: null, postedAt: null, description: null }

// A suggestion is taken up front only when it scores at least this much (the strongest signals are a heading that agrees with the page's
// title, a value that equals what the page states, a machine date). Below it the field starts as "not chosen": a wrong guess that looks
// chosen is worse than an empty row.
export const AUTO_PICK_SCORE = 5

// A description box is taken up front when it is the best by this margin (it is the one required field, so a start helps).
export const AUTO_DESCRIPTION_SCORE = 4

export function pickFromRanked(r: Ranked): Pick {
  return { selector: r.selector, attribute: r.attribute, value: r.value, from: 'suggestion' }
}

export function pickFromElement(e: PageElement): Pick {
  return { selector: e.selector, attribute: '', value: e.text, from: 'element' }
}

// The starting picks for a loaded page: the stored instructions when the link has some this screen can show, else the best suggestions
// that are good enough.
export function startingPicks(page: PickCandidates): Picks {
  const stored = page.current.choices
  const picks: Picks = { ...NO_PICKS }
  if (stored) {
    for (const f of stored.fields) picks[f.label] = { selector: f.selector, attribute: f.attribute ?? '', value: '', from: 'stored' }
    if (stored.descriptionSelector) picks.description = { selector: stored.descriptionSelector, attribute: '', value: '', from: 'stored' }
    return picks
  }
  for (const f of DATA_FIELDS) {
    const top = page.ranked[f].at(0)
    if (top && top.score >= AUTO_PICK_SCORE && top.value.trim() !== '') picks[f] = pickFromRanked(top)
  }
  const box = page.ranked.description.at(0)
  if (box && box.score >= AUTO_DESCRIPTION_SCORE) picks.description = pickFromRanked(box)
  return picks
}

// The choices the backend builds the instructions from: only fields that have a pick, in a fixed order; the attribute only when it is not
// the element's text.
export function toChoices(picks: Picks, rule: RuleDraft | null = null): Choices {
  const fields: FieldChoice[] = []
  for (const f of DATA_FIELDS) {
    const p = picks[f]
    if (!p) continue
    fields.push(
      p.attribute && p.attribute !== 'text'
        ? { label: f, selector: p.selector, attribute: p.attribute }
        : { label: f, selector: p.selector },
    )
  }
  if (rule) {
    const r = ruleToChoice(rule)
    return r ? { fields, description: r } : { fields }
  }
  const d = picks.description
  return d ? { fields, descriptionSelector: d.selector } : { fields }
}

// Whether the choices are worth sending for a preview: the description is the one required field.
export function canPreview(picks: Picks, rule: RuleDraft | null = null): boolean {
  return rule !== null ? ruleToChoice(rule) !== null : picks.description !== null
}

// A value as a screen shows it: one line, cut to n characters with a mark.
export function shortValue(s: string, n = 90): string {
  const one = s.replace(/\s+/g, ' ').trim()
  return one.length > n ? `${one.slice(0, n)}…` : one
}

// The reasons of a suggestion as one line for a tooltip.
export function whyText(r: Ranked): string {
  return r.why.length > 0 ? r.why.join('; ') : 'no particular reason'
}

// What the sample page offers for a field: the ranked suggestions that are worth showing (not empty, at most n).
export function shownSuggestions(page: PickCandidates, field: PickField, n = 5): Ranked[] {
  return page.ranked[field].filter((r) => r.value.trim() !== '').slice(0, n)
}

// The text elements matching a search (case-insensitive, on the text and the selector), capped. An empty search shows the first ones.
export function filterElements(elements: PageElement[], query: string, cap = 40): PageElement[] {
  const q = query.trim().toLowerCase()
  const hit = q === '' ? elements : elements.filter((e) => e.text.toLowerCase().includes(q) || e.selector.toLowerCase().includes(q))
  return hit.slice(0, cap)
}

// Whether two picks read the same place.
export function samePick(a: Pick | null, b: Pick | null): boolean {
  if (a === null || b === null) return a === b
  return a.selector === b.selector && (a.attribute || 'text') === (b.attribute || 'text')
}

// A note when the stored instructions hold things this screen cannot show: saving what is picked here would drop them.
export function lossyNote(lossy: string[]): string {
  if (lossy.length === 0) return ''
  return `The instructions this link has now also hold ${lossy.join(', ')}, which this screen cannot show: saving what is picked here would drop that.`
}

// The words for a failed load: the message, with the Browser's advice when the site stopped the page.
export function describePickFailure(message: string, advice: string): string {
  return advice ? `${message} ${advice}` : message
}

// --- the description rule being edited ---

export const DEFAULT_LIMIT = 60000
export const MIN_LIMIT = 1000
export const MAX_LIMIT = 200000
export const MAX_PARTS = 8
export const MAX_LIST = 20
export const MAX_CUT_AFTER = 10

export type HeadingMode = 'none' | 'auto' | 'selector'

// One part as it is edited: every key present (the backend's rule leaves empty ones out).
export interface PartDraft {
  include: string
  heading: HeadingMode
  headingSelector: string
  exclude: string[]
  cutAfter: string[]
}

// A rule as it is edited: the limit is the text of a number field until it is valid.
export interface RuleDraft {
  format: 'plain' | 'structured'
  limitText: string
  parts: PartDraft[]
}

export function newPart(include = ''): PartDraft {
  return { include, heading: 'none', headingSelector: '', exclude: [], cutAfter: [] }
}

// A rule that reads the box a person already chose, as one part.
export function draftFromBox(selector: string): RuleDraft {
  return { format: 'plain', limitText: String(DEFAULT_LIMIT), parts: [newPart(selector)] }
}

// A stored rule read back into a draft (defaults filled).
export function draftFromRule(rule: DescriptionRule): RuleDraft {
  return {
    format: rule.format === 'structured' ? 'structured' : 'plain',
    limitText: String(rule.limit && rule.limit > 0 ? rule.limit : DEFAULT_LIMIT),
    parts: rule.parts.map((p: RulePart) => ({
      include: p.include,
      heading: p.heading ?? 'none',
      headingSelector: p.headingSelector ?? '',
      exclude: p.exclude ?? [],
      cutAfter: p.cutAfter ?? [],
    })),
  }
}

// The stored rule for the draft (keys that are empty or default are left out), or null while the limit is not a number in range.
export function ruleToChoice(d: RuleDraft): DescriptionRule | null {
  const limit = parseLimit(d.limitText)
  if (limit === null) return null
  return {
    format: d.format,
    ...(limit !== DEFAULT_LIMIT ? { limit } : {}),
    parts: d.parts.map((p) => ({
      include: p.include.trim(),
      ...(p.heading !== 'none' ? { heading: p.heading } : {}),
      ...(p.heading === 'selector' ? { headingSelector: p.headingSelector.trim() } : {}),
      ...(p.exclude.length > 0 ? { exclude: p.exclude } : {}),
      ...(p.cutAfter.length > 0 ? { cutAfter: p.cutAfter } : {}),
    })),
  }
}

// The limit as typed: empty means the default; otherwise a whole number from MIN_LIMIT to MAX_LIMIT (null when it is not).
export function parseLimit(text: string): number | null {
  const t = text.trim()
  if (t === '') return DEFAULT_LIMIT
  if (!/^\d+$/.test(t)) return null
  const n = Number(t)
  return n >= MIN_LIMIT && n <= MAX_LIMIT ? n : null
}

// What is wrong with the draft that the person can see at once (the backend is the one that decides; this only avoids a useless call).
export function ruleProblems(d: RuleDraft): string[] {
  const out: string[] = []
  if (parseLimit(d.limitText) === null) out.push(`The limit is a whole number from ${MIN_LIMIT} to ${MAX_LIMIT}.`)
  if (d.parts.length === 0) out.push('A rule needs at least one part.')
  d.parts.forEach((p, i) => {
    if (p.include.trim() === '') out.push(`Part ${i + 1} has no box yet.`)
    if (p.heading === 'selector' && p.headingSelector.trim() === '')
      out.push(`Part ${i + 1} takes its heading from an element, but none is given.`)
  })
  return out
}

export function addPart(d: RuleDraft, include = ''): RuleDraft {
  return d.parts.length >= MAX_PARTS ? d : { ...d, parts: [...d.parts, newPart(include)] }
}

// The rule the link's stored instructions already have, as a draft (null when it has none).
export function startingRule(page: PickCandidates): RuleDraft | null {
  const stored = page.current.choices?.description
  return stored ? draftFromRule(stored) : null
}

// A rule always keeps at least one part.
export function removePart(d: RuleDraft, index: number): RuleDraft {
  if (d.parts.length <= 1 || index < 0 || index >= d.parts.length) return d
  return { ...d, parts: d.parts.filter((_, i) => i !== index) }
}

export function movePart(d: RuleDraft, index: number, by: -1 | 1): RuleDraft {
  const to = index + by
  if (index < 0 || index >= d.parts.length || to < 0 || to >= d.parts.length) return d
  const parts = [...d.parts]
  ;[parts[index], parts[to]] = [parts[to], parts[index]]
  return { ...d, parts }
}

export function updatePart(d: RuleDraft, index: number, patch: Partial<PartDraft>): RuleDraft {
  if (index < 0 || index >= d.parts.length) return d
  return { ...d, parts: d.parts.map((p, i) => (i === index ? { ...p, ...patch } : p)) }
}

// A value added to a list (excludes, cut-after texts): trimmed, not empty, not already there, not past the cap.
export function addToList(list: string[], value: string, max: number): string[] {
  const v = value.trim()
  if (v === '' || list.includes(v) || list.length >= max) return list
  return [...list, v]
}

export function removeFromList(list: string[], index: number): string[] {
  return list.filter((_, i) => i !== index)
}

// The suggestions for a box that are not in the list yet.
export function unusedSuggestions(suggested: string[], used: string[]): string[] {
  return suggested.filter((s) => !used.includes(s))
}

// Two parts that take the same box would take its text twice.
export function duplicateBoxes(d: RuleDraft): string[] {
  const seen = new Set<string>()
  const dup = new Set<string>()
  for (const p of d.parts) {
    const k = p.include.trim()
    if (k === '') continue
    if (seen.has(k)) dup.add(k)
    seen.add(k)
  }
  return [...dup]
}

// A size as a person reads it: 1,234 characters.
export function chars(n: number): string {
  return `${n.toLocaleString('en-US')} characters`
}

// The suggestions the page has for a description box (by its selector), or undefined when it has none. Own keys only: a selector such as
// "constructor" must not find something on the object's prototype.
export function descriptionHelpFor(page: PickCandidates, selector: string): DescriptionHelp | undefined {
  return Object.prototype.hasOwnProperty.call(page.descriptionHelp, selector) ? page.descriptionHelp[selector] : undefined
}
