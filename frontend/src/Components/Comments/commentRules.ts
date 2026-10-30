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

// How many levels of replies the server allows (a top-level comment is depth 0).
export const MAX_DEPTH = 5

// How many indentation steps a comment is drawn at: its depth, kept within what the
// server allows so a bad value can never push a row off the screen.
export function indentLevel(depth: number): number {
  if (!Number.isFinite(depth)) return 0
  return Math.max(0, Math.min(MAX_DEPTH, Math.floor(depth)))
}

// Whether a comment can be replied to: not one shown only for context, and not at the
// deepest level.
export function canReplyTo(c: Pick<Comment, 'depth' | 'context'>): boolean {
  return !c.context && c.depth < MAX_DEPTH
}

type Threaded = Pick<Comment, 'id' | 'depth'>

// Where a comment's subtree ends: the index after the last comment below it (the
// comments are in display order, so its replies are the deeper run right after it).
function subtreeEnd(list: Threaded[], index: number): number {
  let end = index + 1
  while (end < list.length && list[end].depth > list[index].depth) end++
  return end
}

// Puts a new reply after the last comment of its parent's subtree (so it is the
// parent's newest reply, as the server orders them). A parent that is not shown puts
// it at the top.
export function insertReply<T extends Threaded & { parentId: string }>(list: T[], reply: T): T[] {
  const at = list.findIndex((c) => c.id === reply.parentId)
  if (at === -1) return [reply, ...list]
  const end = subtreeEnd(list, at)
  return [...list.slice(0, end), reply, ...list.slice(end)]
}

// Drops a comment and everything below it.
export function removeThread<T extends Threaded>(list: T[], id: string): T[] {
  const at = list.findIndex((c) => c.id === id)
  if (at === -1) return list
  return [...list.slice(0, at), ...list.slice(subtreeEnd(list, at))]
}

// How many replies are shown below a comment, at any depth.
export function replyCount(list: Threaded[], id: string): number {
  const at = list.findIndex((c) => c.id === id)
  return at === -1 ? 0 : subtreeEnd(list, at) - at - 1
}

// The delete question: says how many replies go with the comment. When a filter is on
// the replies shown are only some of them, so the wording does not promise a number.
export function deleteMessage(shownReplies: number, filtered: boolean): string {
  if (filtered) return 'Delete this comment and any replies to it? This cannot be undone.'
  if (shownReplies === 0) return 'Delete this comment? This cannot be undone.'
  return `Delete this comment and its ${shownReplies} ${shownReplies === 1 ? 'reply' : 'replies'}? This cannot be undone.`
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
