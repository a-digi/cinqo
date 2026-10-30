// Pure rules for the comments screen: text limits, relative times, colours,
// filters and the options a tool passes when it mounts the panel. No React, so each
// can be script-checked on its own. See plan/ai/comments/comments-engine.md.
import type { Comment } from '../../api/comments'

export const MAX_BODY = 4000
export const PAGE_SIZE = 20

// Characters, not UTF-16 units, as the server counts them.
export function charsUsed(text: string): number {
  return Array.from(text.trim()).length
}

// Why this text cannot be sent, or null when it can.
export function bodyProblem(text: string): string | null {
  const n = charsUsed(text)
  if (n === 0) return 'Write something first.'
  if (n > MAX_BODY) return `Too long: ${n.toLocaleString('en')} of ${MAX_BODY.toLocaleString('en')} characters.`
  return null
}

export function isEdited(c: Pick<Comment, 'updatedAt'>): boolean {
  return c.updatedAt !== ''
}

export function authorLabel(c: Pick<Comment, 'authorName'>): string {
  return c.authorName.trim() || 'User'
}

// "just now", "5m ago", "3h ago", "2d ago", then a plain date after 30 days. An
// unparseable timestamp gives "".
export function relativeTime(now: Date, iso: string): string {
  const then = new Date(iso)
  if (Number.isNaN(then.getTime())) return ''
  const seconds = Math.floor((now.getTime() - then.getTime()) / 1000)
  if (seconds < 60) return 'just now'
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return `${minutes}m ago`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours}h ago`
  const days = Math.floor(hours / 24)
  if (days < 30) return `${days}d ago`
  return then.toLocaleDateString('en-GB', { day: 'numeric', month: 'short', year: 'numeric' })
}

// Appends an older page to what is already shown, dropping anything already there.
export function mergeOlder(existing: Comment[], older: Comment[]): Comment[] {
  const seen = new Set(existing.map((c) => c.id))
  return [...existing, ...older.filter((c) => !seen.has(c.id))]
}

// --- colours -----------------------------------------------------------------

export interface Accent {
  from: string
  to: string
  soft: string
  text: string
}

// The same palette as the Home dashboard, so an author keeps a colour everywhere.
export const ACCENTS: Accent[] = [
  { from: '#60a5fa', to: '#2563eb', soft: '#eff6ff', text: '#1d4ed8' },
  { from: '#4ade80', to: '#16a34a', soft: '#f0fdf4', text: '#15803d' },
  { from: '#fbbf24', to: '#d97706', soft: '#fffbeb', text: '#b45309' },
  { from: '#fb7185', to: '#e11d48', soft: '#fff1f2', text: '#be123c' },
  { from: '#a78bfa', to: '#7c3aed', soft: '#f5f3ff', text: '#6d28d9' },
  { from: '#2dd4bf', to: '#0d9488', soft: '#f0fdfa', text: '#0f766e' },
  { from: '#38bdf8', to: '#0284c7', soft: '#f0f9ff', text: '#0369a1' },
  { from: '#fb923c', to: '#ea580c', soft: '#fff7ed', text: '#c2410c' },
]

// The panel's own accent (violet, like the Home "conversations" card).
export const PANEL_ACCENT: Accent = ACCENTS[4]

export function hashString(s: string): number {
  let h = 0
  for (const ch of s) h = (h * 31 + (ch.codePointAt(0) ?? 0)) >>> 0
  return h
}

// An author always gets the same colour: from their id, never their (changeable) name.
export function accentForAuthor(authorId: string): Accent {
  return ACCENTS[hashString(authorId) % ACCENTS.length]
}

export function initialOf(text: string): string {
  const first = Array.from(text.trim())[0]
  return first ? first.toUpperCase() : '•'
}

// --- filters -----------------------------------------------------------------

export interface FilterState {
  mine: boolean
  from: string // YYYY-MM-DD or ''
  to: string // YYYY-MM-DD or ''
}

export const NO_FILTERS: FilterState = { mine: false, from: '', to: '' }

export function hasFilters(f: FilterState): boolean {
  return f.mine || f.from !== '' || f.to !== ''
}

const DATE = /^(\d{4})-(\d{2})-(\d{2})$/

// Local midnight of a YYYY-MM-DD day, or null if it is not a real date. The user
// means their own day, so the bounds are local midnights sent as UTC instants.
function localMidnight(day: string, addDays: number): Date | null {
  const m = DATE.exec(day)
  if (!m) return null
  const [y, mo, d] = [Number(m[1]), Number(m[2]), Number(m[3])]
  const date = new Date(y, mo - 1, d + addDays)
  const real = new Date(y, mo - 1, d)
  if (real.getFullYear() !== y || real.getMonth() !== mo - 1 || real.getDate() !== d) return null
  return date
}

// The start of that day (inclusive bound), as an ISO timestamp.
export function dayStartISO(day: string): string | undefined {
  return localMidnight(day, 0)?.toISOString()
}

// The start of the NEXT day: the server's createdTo is exclusive, so picking "to
// 5 Oct" means "up to and including all of 5 Oct".
export function dayEndExclusiveISO(day: string): string | undefined {
  return localMidnight(day, 1)?.toISOString()
}

// What the API's query needs for these filters. userId is the signed-in user (for "mine").
export function filterParams(f: FilterState, userId: string): { authorId?: string; createdFrom?: string; createdTo?: string } {
  const out: { authorId?: string; createdFrom?: string; createdTo?: string } = {}
  if (f.mine && userId) out.authorId = userId
  const from = f.from ? dayStartISO(f.from) : undefined
  const to = f.to ? dayEndExclusiveISO(f.to) : undefined
  if (from) out.createdFrom = from
  if (to) out.createdTo = to
  return out
}

// Why the date range cannot be applied, or null.
export function rangeProblem(f: FilterState): string | null {
  if (f.from && !dayStartISO(f.from)) return 'The start date is not valid.'
  if (f.to && !dayStartISO(f.to)) return 'The end date is not valid.'
  if (f.from && f.to && f.from > f.to) return 'The start date is after the end date.'
  return null
}

// --- mounting ----------------------------------------------------------------

export interface MountOptions {
  owner: string
  entityType: string
  entityId: string
  title?: string
}

const NAME = /^[a-z][a-z0-9_]*$/
const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i

// Why a tool's mount options are unusable, or null. The server checks all of this
// again; this just fails early, with a message a tool author can act on.
export function mountOptionsProblem(o: Partial<MountOptions> | null | undefined): string | null {
  if (!o || typeof o !== 'object') return 'options are required'
  if (typeof o.owner !== 'string' || o.owner.length > 64 || !NAME.test(o.owner))
    return "owner must be the tool's slug (lower-case letters, digits, underscores)"
  if (typeof o.entityType !== 'string' || o.entityType.length > 40 || !NAME.test(o.entityType))
    return 'entityType must be a lower-case name such as "job"'
  if (typeof o.entityId !== 'string' || !UUID.test(o.entityId)) return 'entityId must be a UUID'
  if (o.title !== undefined && typeof o.title !== 'string') return 'title must be a string'
  return null
}
