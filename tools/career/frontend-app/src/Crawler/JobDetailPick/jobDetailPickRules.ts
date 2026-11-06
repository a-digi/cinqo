// jobDetailPickRules.ts (plan 186, step 6) — the pure rules of the picking screen: what a pick is, how picks become the choices the backend
// builds the instructions from, which suggestion is taken up front, and how the page's text elements are filtered. No React, no calls.
import type { Choices, FieldChoice, PageElement, PickCandidates, PickField, Ranked } from './jobDetailPick'

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
export function toChoices(picks: Picks): Choices {
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
  const d = picks.description
  return d ? { fields, descriptionSelector: d.selector } : { fields }
}

// Whether the choices are worth sending for a preview: the description is the one required field.
export function canPreview(picks: Picks): boolean {
  return picks.description !== null
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
