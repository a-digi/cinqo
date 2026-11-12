// localPieces.ts — the small questions the local driver of Re-suggest puts to a model that cannot call tools (plan 268, step 4), and the checks of its answers. Like the import's own
// pieces (codeImport/pieces.ts) each question holds only the text it needs and the JSON shape wanted, and its answer is checked before anything is changed: a field that is not text,
// too long, a date that is not a date, a description that lost a bullet point, is refused (and asked again once with the exact fault). Nothing here is imported from the remote path.
import type { Piece, Verdict } from '../../../../Cinqo/Conversation/toolFree/askPiece'
import { wishOf } from '../resuggestRules'

const ok = <T>(value: T): Verdict<T> => ({ ok: true, value })
const bad = (reason: string): Verdict<never> => ({ ok: false, reason })

const norm = (s: string): string => s.toLowerCase().replace(/\s+/g, ' ').trim()
const tokens = (s: string): string[] => norm(s).match(/[\p{L}\p{N}&+#]+/gu) ?? []

// The text given to a small model is bounded.
export const clip = (s: string, max: number): string => (s.length <= max ? s : `${s.slice(0, max)} …`)
const block = (label: string, text: string): string => `${label}:\n"""\n${text}\n"""`

const asObject = (parsed: unknown): Record<string, unknown> | null => {
  const value = Array.isArray(parsed) && parsed.length === 1 ? (parsed[0] as unknown) : parsed
  return typeof value === 'object' && value !== null && !Array.isArray(value) ? (value as Record<string, unknown>) : null
}

// ---- the skills of a whole section: what to add and what to take away

export interface SkillsDiff {
  add: string[]
  remove: string[]
}

export const MAX_SKILL_LENGTH = 40
export const MAX_SKILLS_TOTAL = 200

export function skillsDiffPiece(current: string[], cvSkills: string, wish: string): Piece<SkillsDiff> {
  const now = new Set(current.map(norm))
  const shape = '{"add":["<skill to add>"],"remove":["<skill exactly as it is now suggested>"]}'
  return {
    prompt: `${cvSkills.trim() === '' ? '' : `${block('Skills text of the CV', clip(cvSkills, 2500))}\n`}The skills now suggested: ${current.length === 0 ? '(none)' : current.join('; ')}\nThe person's wish: "${wishOf(wish)}"\nChange the suggested skills as the wish says. When the wish is general, add the skills the CV lists that are missing and take away what is not a skill. A skill is one to four words, such as "PHP" or "Domain Driven Design". Answer with this JSON and nothing else: ${shape}`,
    validate: (parsed) => {
      const o = asObject(parsed)
      if (o === null) return bad(`it must be a JSON object ${shape}`)
      const list = (v: unknown): unknown[] | null => (v === undefined || v === null ? [] : Array.isArray(v) ? v : null)
      const addRaw = list(o.add)
      const removeRaw = list(o.remove)
      if (addRaw === null || removeRaw === null) return bad('"add" and "remove" must be lists of skills')
      const seen = new Set<string>()
      const add: string[] = []
      for (const item of addRaw) {
        if (typeof item !== 'string') continue
        const s = item.replace(/\s+/g, ' ').trim()
        if (s === '' || s.length > MAX_SKILL_LENGTH || tokens(s).length > 4 || /[.!?]$/.test(s)) continue
        if (now.has(norm(s)) || seen.has(norm(s))) continue
        seen.add(norm(s))
        add.push(s)
      }
      const removeSeen = new Set<string>()
      const remove: string[] = []
      for (const item of removeRaw) {
        if (typeof item !== 'string') continue
        const key = norm(item)
        // only a skill that is suggested now can be taken away
        if (now.has(key) && !removeSeen.has(key)) {
          removeSeen.add(key)
          remove.push(current.find((c) => norm(c) === key) ?? item)
        }
      }
      if (current.length - remove.length + add.length > MAX_SKILLS_TOTAL)
        return bad(`there can be at most ${String(MAX_SKILLS_TOTAL)} skills`)
      return ok({ add, remove })
    },
    fallback: () => null,
  }
}

// ---- the fields of one suggestion (a section, a job, an education entry, one skill)

export type FieldKind = 'short' | 'date' | 'long'
export interface FieldSpec {
  key: string
  kind: FieldKind
  // May the model leave it empty (a date, a degree) or must it hold something.
  required: boolean
}

const MAX_SHORT = 200
const MAX_LONG = 4000
const DATE = /^\d{4}(-\d{2}(-\d{2})?)?$/

export const FIELD_SPECS = {
  profile: [{ key: 'name', kind: 'short', required: true }],
  persona: [
    { key: 'name', kind: 'short', required: true },
    { key: 'title', kind: 'short', required: true },
  ],
  personalDetails: [
    { key: 'headline', kind: 'short', required: false },
    { key: 'summary', kind: 'long', required: false },
    { key: 'location', kind: 'short', required: false },
    { key: 'desiredTitles', kind: 'short', required: false },
    { key: 'desiredLocations', kind: 'short', required: false },
  ],
  skills: [{ key: 'value', kind: 'short', required: true }],
  experience: [
    { key: 'company', kind: 'short', required: true },
    { key: 'title', kind: 'short', required: true },
    { key: 'startDate', kind: 'date', required: false },
    { key: 'endDate', kind: 'date', required: false },
    { key: 'description', kind: 'long', required: true },
  ],
  education: [
    { key: 'institution', kind: 'short', required: true },
    { key: 'degree', kind: 'short', required: false },
    { key: 'fieldOfStudy', kind: 'short', required: false },
    { key: 'startDate', kind: 'date', required: false },
    { key: 'endDate', kind: 'date', required: false },
    { key: 'description', kind: 'long', required: false },
  ],
} satisfies Record<string, FieldSpec[]>

const bulletCount = (t: string): number => t.split('\n').filter((l) => /^\s*[-•*]\s/.test(l)).length

// What a field says is checked: the kind, the length, the date shape, and for a long text that no bullet point was lost.
function checkField(spec: FieldSpec, before: string, value: unknown): Verdict<string> {
  if (typeof value !== 'string') return bad(`"${spec.key}" must be text`)
  const t = spec.kind === 'long' ? value.replace(/\r/g, '').trim() : value.replace(/\s+/g, ' ').trim()
  if (t === '') return spec.required ? bad(`"${spec.key}" must not be empty`) : ok('')
  if (spec.kind === 'date') return DATE.test(t) ? ok(t) : bad(`"${spec.key}" must be YYYY, YYYY-MM or YYYY-MM-DD`)
  const max = spec.kind === 'long' ? MAX_LONG : MAX_SHORT
  if (t.length > max) return bad(`"${spec.key}" is too long (${String(t.length)} characters, at most ${String(max)})`)
  if (spec.kind === 'long' && bulletCount(t) < bulletCount(before))
    return bad(`"${spec.key}" lost bullet points (it had ${String(bulletCount(before))}, now ${String(bulletCount(t))}): keep every one`)
  return ok(t)
}

// The question about one suggestion: the model gets the suggestion as JSON (the fields it may change), the CV text that belongs to it (or none), and the wish, and answers with the
// whole suggestion; the value of the piece is only the fields that really changed.
export function fieldsPiece(
  specs: FieldSpec[],
  current: Record<string, string>,
  cvExcerpt: string,
  wish: string,
): Piece<Record<string, string>> {
  const shape = JSON.stringify(Object.fromEntries(specs.map((s) => [s.key, `<${s.key}>`])))
  return {
    prompt: `${cvExcerpt.trim() === '' ? '' : `${block('The part of the CV this suggestion comes from', clip(cvExcerpt, 3000))}\n`}${block('The suggestion now (JSON)', JSON.stringify(Object.fromEntries(specs.map((s) => [s.key, current[s.key] ?? '']))))}\nThe person's wish: "${wishOf(wish)}"\nGive the suggestion again with the changes the wish asks for. Check it against the CV text when the wish is about what the CV says. Keep every field that needs no change exactly as it is, keep every bullet point (a line starting with "- ") of a text, keep the CV's language, and add nothing the CV does not say. Dates are YYYY, YYYY-MM or YYYY-MM-DD. Answer with this JSON and nothing else: ${shape}`,
    validate: (parsed) => {
      const o = asObject(parsed)
      if (o === null) return bad(`it must be a JSON object ${shape}`)
      const changed: Record<string, string> = {}
      for (const spec of specs) {
        if (!(spec.key in o)) continue
        const before = current[spec.key] ?? ''
        const v = checkField(spec, before, o[spec.key])
        if (!v.ok) return v
        if (v.value !== before.trim()) changed[spec.key] = v.value
      }
      return ok(changed)
    },
    fallback: () => null,
  }
}
