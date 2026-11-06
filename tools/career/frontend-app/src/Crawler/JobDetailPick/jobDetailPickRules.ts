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
  PickFlag,
  PickTest,
  PickTestJob,
  Ranked,
  RulePart,
  StructuredHint,
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
  // 'suggestion' (one of the ranked candidates), 'element' (picked from the list of text elements), 'structured' (the page's own data) or
  // 'stored' (read back from the instructions the link already has).
  from: 'suggestion' | 'element' | 'stored' | 'structured'
  // A pick of the page's structured data (JSON-LD, meta tags) has a source and a path and no selector or attribute.
  source?: string
  path?: string
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

// A pick of what the page itself states (its JobPosting JSON-LD or a meta tag): read by source and path, not by a selector.
export function pickFromHint(h: StructuredHint): Pick {
  return { selector: '', attribute: '', value: h.value, from: 'structured', source: h.source, path: h.path }
}

export function isStructuredPick(p: Pick | null): boolean {
  return (p?.source ?? '') !== ''
}

// The structured hints a field can be picked from: only while the installed Browser tool can read them, and never a meta tag as a
// start (a page's og:title carries the site and the company next to the title).
export function hintsFor(page: PickCandidates, field: PickField): StructuredHint[] {
  return page.structuredSupported
    ? page.structuredHints.filter((h) => h.field === field && h.value.trim() !== '' && field !== 'description')
    : []
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
    for (const f of stored.fields) {
      picks[f.label] = f.source
        ? { selector: '', attribute: '', value: '', from: 'stored', source: f.source, path: f.path ?? '' }
        : { selector: f.selector ?? '', attribute: f.attribute ?? '', value: '', from: 'stored' }
    }
    if (stored.descriptionSelector) picks.description = { selector: stored.descriptionSelector, attribute: '', value: '', from: 'stored' }
    return picks
  }
  for (const f of DATA_FIELDS) {
    // What the page itself states (JSON-LD) is the most robust start: it survives a redesign. Otherwise a strong suggestion.
    const stated = hintsFor(page, f).find((h) => h.source === 'jsonld')
    if (stated) {
      picks[f] = pickFromHint(stated)
      continue
    }
    const top = page.ranked[f].at(0)
    if (top && top.score >= AUTO_PICK_SCORE && top.value.trim() !== '') picks[f] = pickFromRanked(top)
  }
  const box = page.ranked.description.at(0)
  if (box && box.score >= AUTO_DESCRIPTION_SCORE) picks.description = pickFromRanked(box)
  return picks
}

// The choices the backend builds the instructions from: only fields that have a pick, in a fixed order; the attribute only when it is not
// the element's text.
export function toChoices(picks: Picks, rule: RuleDraft | null = null, replaceTitle = false): Choices {
  const fields: FieldChoice[] = []
  for (const f of DATA_FIELDS) {
    const p = picks[f]
    if (!p) continue
    if (isStructuredPick(p)) {
      fields.push({ label: f, source: p.source, path: p.path ?? '' })
      continue
    }
    fields.push(
      p.attribute && p.attribute !== 'text'
        ? { label: f, selector: p.selector, attribute: p.attribute }
        : { label: f, selector: p.selector },
    )
  }
  const extra = replaceTitle && picks.title !== null ? { replaceTitle: true } : {}
  if (rule) {
    const r = ruleToChoice(rule)
    return r ? { fields, description: r, ...extra } : { fields, ...extra }
  }
  const d = picks.description
  return d ? { fields, descriptionSelector: d.selector, ...extra } : { fields, ...extra }
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
  if (isStructuredPick(a) || isStructuredPick(b)) return a.source === b.source && a.path === b.path
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
  // A box (include) or, for the text the page states in its structured data, a source and path (then include is empty).
  include: string
  source: string
  path: string
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
  return { include, source: '', path: '', heading: 'none', headingSelector: '', exclude: [], cutAfter: [] }
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
      include: p.include ?? '',
      source: p.source ?? '',
      path: p.path ?? '',
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
      ...(p.source !== '' ? { source: p.source, path: p.path } : { include: p.include.trim() }),
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
    if (p.source === '' && p.include.trim() === '') out.push(`Part ${i + 1} has no box yet.`)
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
    const k = p.source !== '' ? `${p.source}:${p.path}` : p.include.trim()
    if (k === '' || k === ':') continue
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

// --- checking the picks on sample jobs, and saving ---

// A stable text for choices: two checks of the same choices have the same key, any change makes another.
export function choicesKey(choices: Choices): string {
  return JSON.stringify(choices)
}

// What was checked and for which choices.
export interface Verification {
  key: string
  test: PickTest
}

export type VerificationState = 'none' | 'stale' | 'fresh'

// Whether the check on sample jobs is still about the choices on the screen.
export function verificationState(v: Verification | null, currentKey: string): VerificationState {
  if (v === null) return 'none'
  return v.key === currentKey ? 'fresh' : 'stale'
}

export interface FlagGroups {
  blocking: PickFlag[]
  warnings: PickFlag[]
  info: PickFlag[]
}

export function splitFlags(flags: PickFlag[]): FlagGroups {
  return {
    blocking: flags.filter((f) => f.severity === 'blocking'),
    warnings: flags.filter((f) => f.severity === 'warning'),
    info: flags.filter((f) => f.severity === 'info'),
  }
}

// The jobs of a check whose page could be read.
export function checkedJobs(test: PickTest): PickTestJob[] {
  return test.jobs.filter((j) => !j.error)
}

// How many of the checked jobs would get a different description than the one they have.
export function changedCount(test: PickTest): number {
  return checkedJobs(test).filter((j) => j.diff && !j.diff.unchanged).length
}

export interface SaveInput {
  previewValid: boolean
  verification: VerificationState
  test: PickTest | null
  // Whether the link has saved jobs to check the picks on.
  hasSamples: boolean
  // The person's tick: "I looked at the warnings" (after a check), or "save without checking" (a link without saved jobs).
  confirmed: boolean
  saving: boolean
}

export interface SaveGate {
  ok: boolean
  // Why saving is not possible yet ("" when it is).
  reason: string
}

// Saving needs valid instructions and, when the link has jobs to check on, a fresh check without a blocking flag; warnings (and a link
// without jobs) need the person's tick.
export function saveGate(i: SaveInput): SaveGate {
  if (i.saving) return { ok: false, reason: 'Saving…' }
  if (!i.previewValid) return { ok: false, reason: 'Choose where the description is: the instructions are not valid yet.' }
  if (!i.hasSamples) {
    return i.confirmed
      ? { ok: true, reason: '' }
      : { ok: false, reason: 'This link has no saved jobs to check the picks on: tick the box to save without checking.' }
  }
  if (i.verification === 'none' || i.test === null) return { ok: false, reason: 'Check the picks on the sample jobs first.' }
  if (i.verification === 'stale') return { ok: false, reason: 'The picks changed since the check: check them again.' }
  if (checkedJobs(i.test).length === 0) return { ok: false, reason: 'None of the sample pages could be read: check again.' }
  const groups = splitFlags(i.test.flags)
  if (groups.blocking.length > 0) return { ok: false, reason: groups.blocking[0].message }
  if (groups.warnings.length > 0 && !i.confirmed) return { ok: false, reason: 'Look at the warnings, then tick the box to save.' }
  return { ok: true, reason: '' }
}

// What a recrawl could do, said plainly: how many of the checked jobs would change, how many jobs the link has, and that nothing is
// recrawled by itself.
export function recrawlNote(checked: number, changed: number, jobCount: number): string {
  const jobs = (n: number) => `${n} saved job${n === 1 ? '' : 's'}`
  const first =
    checked === 0 ? '' : `Of the ${checked} job${checked === 1 ? '' : 's'} checked, ${changed} would get a different description. `
  return `${first}This link has ${jobs(jobCount)}. A recrawl (“Crawl job details now”) re-scores the jobs whose description changes; nothing is recrawled automatically.`
}

export interface ValueRow {
  field: PickField
  value: string
  // A second line: what the date becomes after cleaning, how long the description is.
  note: string
}

// What one checked job gave, field by field, for the fields that were chosen (and the description).
export function valueRows(job: PickTestJob, chosen: PickField[]): ValueRow[] {
  const rows: ValueRow[] = []
  // a field the page did not give is absent from the values
  const val = (k: string): string => (job.values as Partial<Record<string, string>>)[k] ?? ''
  for (const f of DATA_FIELDS) {
    if (!chosen.includes(f)) continue
    const v = val(f).trim()
    rows.push({
      field: f,
      value: v === '' ? '' : shortValue(v, 160),
      note:
        f === 'postedAt' && v !== '' ? (job.postedAtClean ? `saved as ${job.postedAtClean}` : 'not a date: it would be left empty') : '',
    })
  }
  const d = val('description').trim()
  rows.push({ field: 'description', value: d === '' ? '' : shortValue(d, 160), note: d === '' ? '' : chars(job.descriptionChars) })
  return rows
}

// Whether the stored instructions replace the title (read back so the screen starts from them).
export function startingReplaceTitle(page: PickCandidates): boolean {
  return page.current.choices?.replaceTitle === true
}

// Who wrote a link's job-detail instructions, in words ('' when nothing is known or there are none).
export function originLabel(origin: string): string {
  switch (origin) {
    case 'picked':
      return 'picked on a job page'
    case 'ai':
      return 'written by the AI'
    case 'manual':
      return 'typed by hand'
    case 'inherited':
      return 'copied from another link'
    default:
      return ''
  }
}

// What the instructions do to a job's own data, said for the origin: only picked instructions fill an empty company, location or date.
export function fillNote(origin: string): string {
  return origin === 'picked'
    ? "These instructions fill a job's empty company, location and posted date when its page is crawled; they never replace what a job already has."
    : ''
}

// A part that takes the text the page states in its structured data (its JSON-LD description) instead of a box.
export function newStructuredPart(source = 'jsonld', path = 'description'): PartDraft {
  return { ...newPart(''), source, path }
}

// A rule that reads the description the page states, as one part (in the structured format: the stated text is HTML).
export function draftFromStatedDescription(): RuleDraft {
  return { format: 'structured', limitText: String(DEFAULT_LIMIT), parts: [newStructuredPart()] }
}

// The description the page states, when the installed Browser tool can read it.
export function statedDescription(page: PickCandidates): StructuredHint | undefined {
  return page.structuredSupported ? page.structuredHints.find((h) => h.field === 'description' && h.source === 'jsonld') : undefined
}

// Whether the picks or the rule read anything from the page's structured data.
export function usesStructured(picks: Picks, rule: RuleDraft | null): boolean {
  return DATA_FIELDS.some((f) => isStructuredPick(picks[f])) || (rule?.parts.some((p) => p.source !== '') ?? false)
}

// What a chosen description box also holds that is probably not description (a sidebar, a share bar: the furniture the Browser found inside
// it), said for the description row; "" when there is nothing worth saying. Risky suggestions are not counted.
export function furnitureNote(page: PickCandidates, selector: string): string {
  const box = page.ranked.description.find((r) => r.selector === selector)
  const help = descriptionHelpFor(page, selector)
  if (!box || !help || box.chars <= 0) return ''
  const items = help.excludes.filter((e) => !e.risky)
  const total = items.reduce((n, e) => n + e.chars, 0)
  if (total < 200 || total / box.chars < 0.05) return ''
  const kinds = [...new Set(items.map((e) => e.kind))].slice(0, 3).join(', ')
  return `This box also holds ${chars(total)} of page furniture (${kinds}). “Read the description with a rule” lets you leave it out.`
}
