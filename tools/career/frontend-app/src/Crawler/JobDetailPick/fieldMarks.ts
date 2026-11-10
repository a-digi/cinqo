// fieldMarks.ts (plan 249, step 4) — the pure parts of the page view of the Job details picker: which numbered element of the page copy a pick is, what a click or a
// selection in the copy is about, what to draw on the page and what a pick made on the page looks like.
import type { PickField, ViewElement } from './jobDetailPick'
import { pickFromElement, type Pick, type Picks, type RuleDraft } from './jobDetailPickRules'

export const FIELD_ORDER: readonly PickField[] = ['title', 'company', 'location', 'postedAt', 'description']

// The class an element of the copy carries for the field it is.
export const FIELD_CLASS: Record<PickField, string> = {
  title: 'cinqo-f-title',
  company: 'cinqo-f-company',
  location: 'cinqo-f-location',
  postedAt: 'cinqo-f-postedat',
  description: 'cinqo-f-description',
}

export const FIELD_CLASSES = Object.values(FIELD_CLASS)

// One colour per field: on the page, in the strip's dots and in the bar's buttons.
export const FIELD_COLOR: Record<PickField, string> = {
  title: '#2563eb',
  company: '#7c3aed',
  location: '#d97706',
  postedAt: '#0d9488',
  description: '#16a34a',
}

// What the Job details view adds to the page copy: the field marks (an outline, a tint; a box that wraps blocks is made a block by the frame) and a pointer.
export const FIELD_MARK_STYLE = `
[data-cinqo-e] { cursor: pointer; }
[data-cinqo-e].cinqo-block { display: block !important; }
${FIELD_ORDER.map(
  (f) =>
    `[data-cinqo-e].${FIELD_CLASS[f]} { outline: 3px solid ${FIELD_COLOR[f]}; outline-offset: 2px; background-color: ${FIELD_COLOR[f]}29 !important; border-radius: 2px; }`,
).join('\n')}
`

export function viewByNumber(view: ViewElement[]): Map<number, ViewElement> {
  return new Map(view.map((v) => [v.n, v]))
}

// The element of the copy a selector stands for: the one the analysis verified for it (a unique one first), else what the copy itself answered for it
// (`found`, asked of the document by the view). undefined when the page has no such element.
export function numberOfSelector(view: ViewElement[], selector: string, found?: Map<string, number>): number | undefined {
  if (selector === '') return undefined
  let first: number | undefined
  for (const v of view) {
    if (v.selector !== selector) continue
    if (v.unique) return v.n
    first ??= v.n
  }
  return first ?? found?.get(selector)
}

// The selectors of the picks (and the boxes of a description rule) the analysis does not know, to be asked of the copy itself.
export function unknownSelectors(view: ViewElement[], picks: Picks, rule: RuleDraft | null): string[] {
  const known = new Set(view.map((v) => v.selector))
  const out: string[] = []
  for (const s of selectorsOf(picks, rule)) if (!known.has(s) && !out.includes(s)) out.push(s)
  return out
}

function selectorsOf(picks: Picks, rule: RuleDraft | null): string[] {
  const out: string[] = []
  for (const f of FIELD_ORDER) {
    if (f === 'description' && rule !== null) continue
    const p = picks[f]
    if (p && (p.source ?? '') === '' && p.selector !== '') out.push(p.selector)
  }
  if (rule) for (const part of rule.parts) if (part.include.trim() !== '') out.push(part.include.trim())
  return out
}

// What each element of the copy is now: every field whose pick (or, for the description, whose rule's boxes) is that element.
export function fieldsByNumber(
  view: ViewElement[],
  picks: Picks,
  rule: RuleDraft | null,
  found?: Map<string, number>,
): Map<number, PickField[]> {
  const out = new Map<number, PickField[]>()
  const put = (selector: string, field: PickField) => {
    const n = numberOfSelector(view, selector, found)
    if (n === undefined) return
    const list = out.get(n) ?? []
    if (!list.includes(field)) list.push(field)
    out.set(n, list)
  }
  for (const f of FIELD_ORDER) {
    if (f === 'description' && rule !== null) {
      for (const part of rule.parts) if (part.include.trim() !== '') put(part.include.trim(), 'description')
      continue
    }
    const p = picks[f]
    if (p && (p.source ?? '') === '') put(p.selector, f)
  }
  return out
}

// The class to draw on each number: the last field of an element wins (the description, the box around the others, is drawn first).
export function drawnFields(byNumber: Map<number, PickField[]>): Map<number, string> {
  const out = new Map<number, string>()
  for (const [n, fields] of byNumber) {
    const f = [...fields].sort((a, b) => FIELD_ORDER.indexOf(b) - FIELD_ORDER.indexOf(a)).at(-1)
    if (f) out.set(n, FIELD_CLASS[f])
  }
  return out
}

// The numbered element a click or a selection is about: the one it is in, or the nearest one around it. An element of the page copy belongs to the iframe's
// window, so it is not tested with instanceof.
export function numberedAround(node: Node | null): number | undefined {
  let el: Partial<Element> | null = node === null ? null : node.nodeType === 1 ? (node as Element) : node.parentElement
  if (!el || typeof el.closest !== 'function') return undefined
  el = el.closest('[data-cinqo-e]')
  const v = el?.getAttribute?.('data-cinqo-e')
  const n = v === null || v === undefined ? NaN : Number(v)
  return Number.isInteger(n) && n > 0 ? n : undefined
}

// What a pick made on the page reads: the element's verified selector and its text (the machine date is a later choice).
export function pickFromView(v: ViewElement): Pick {
  return pickFromElement({ selector: v.selector, unique: v.unique, tag: v.tag, text: v.text, chars: v.chars })
}

// The pick of a field when its selector is on the page but several places have it (a stored or suggested selector that is not unique).
export function isAmbiguousPick(view: ViewElement[], pick: Pick | null): boolean {
  if (!pick || (pick.source ?? '') !== '' || pick.selector === '') return false
  const same = view.filter((v) => v.selector === pick.selector)
  return same.length > 0 && same.every((v) => !v.unique)
}
