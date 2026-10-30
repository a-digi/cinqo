import type { AllAiTraceLogsEntry } from '../../../api/conversations'

// The pure rules behind the AI Logs overview table (plan/ai/conversation/step-43-ai-logs-table-redesign.md):
// searching, sorting, grouping by day and the short forms of ids, folders and dates. No React, and every
// date rule takes `now` as a parameter, so they can be script-checked.

export type SortKey = 'newest' | 'oldest' | 'files' | 'title'

const MONTHS = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec']

// A deleted conversation has no title (the API sends none, or an empty one).
export function hasTitle(entry: AllAiTraceLogsEntry): boolean {
  return (entry.title ?? '').trim() !== ''
}

// Plain substring match, never a pattern built from what was typed.
export function filterEntries(entries: AllAiTraceLogsEntry[], query: string): AllAiTraceLogsEntry[] {
  const q = query.trim().toLowerCase()
  if (q === '') return entries
  return entries.filter((e) => [e.title ?? '', e.conversationId, e.folder].some((field) => field.toLowerCase().includes(q)))
}

function time(entry: AllAiTraceLogsEntry): number {
  const t = Date.parse(entry.modifiedAt)
  return Number.isNaN(t) ? Number.NaN : t
}

// Stable: equal rows keep their order. Rows without a usable date go last for both date sorts, and
// deleted conversations go last when sorting by title.
export function sortEntries(entries: AllAiTraceLogsEntry[], key: SortKey): AllAiTraceLogsEntry[] {
  const compare = (a: AllAiTraceLogsEntry, b: AllAiTraceLogsEntry): number => {
    switch (key) {
      case 'files':
        return b.logCount - a.logCount
      case 'title': {
        if (hasTitle(a) !== hasTitle(b)) return hasTitle(a) ? -1 : 1
        return (a.title ?? '').localeCompare(b.title ?? '', undefined, { sensitivity: 'base' })
      }
      case 'newest':
      case 'oldest': {
        const ta = time(a)
        const tb = time(b)
        if (Number.isNaN(ta) || Number.isNaN(tb)) return Number.isNaN(ta) === Number.isNaN(tb) ? 0 : Number.isNaN(ta) ? 1 : -1
        return key === 'newest' ? tb - ta : ta - tb
      }
    }
  }
  return entries
    .map((entry, i) => ({ entry, i }))
    .sort((a, b) => compare(a.entry, b.entry) || a.i - b.i)
    .map(({ entry }) => entry)
}

function startOfDay(d: Date): number {
  return new Date(d.getFullYear(), d.getMonth(), d.getDate()).getTime()
}

function dayDiff(d: Date, now: Date): number {
  // Calendar days, not 24 h blocks: a day with a clock change is 23 or 25 hours long.
  return Math.round((startOfDay(now) - startOfDay(d)) / 86_400_000)
}

function pad(n: number): string {
  return String(n).padStart(2, '0')
}

function dateLabel(d: Date, now: Date): string {
  const base = `${d.getDate()} ${MONTHS[d.getMonth()] ?? ''}`
  return d.getFullYear() === now.getFullYear() ? base : `${base} ${d.getFullYear()}`
}

export interface DayGroup {
  // "Today", "Yesterday", "29 Oct" or "29 Oct 2025"; "Unknown date" for rows without a usable date.
  label: string
  entries: AllAiTraceLogsEntry[]
}

// Keeps the incoming order: a group appears where its first row appears, so a newest-first list yields
// Today, Yesterday, older. Sorted by anything else, a day simply collects its rows under its first one.
export function groupByDay(entries: AllAiTraceLogsEntry[], now: Date): DayGroup[] {
  const groups: DayGroup[] = []
  const index = new Map<string, DayGroup>()
  for (const entry of entries) {
    const t = time(entry)
    let label = 'Unknown date'
    if (!Number.isNaN(t)) {
      const d = new Date(t)
      const diff = dayDiff(d, now)
      label = diff === 0 ? 'Today' : diff === 1 ? 'Yesterday' : dateLabel(d, now)
    }
    let group = index.get(label)
    if (!group) {
      group = { label, entries: [] }
      index.set(label, group)
      groups.push(group)
    }
    group.entries.push(entry)
  }
  return groups
}

// "Just now", "12 min ago", "3 h ago", then "Today, 09:05", "Yesterday, 20:47", "29 Oct, 14:41" and
// "29 Oct 2025, 14:41". "—" when the date is missing or unreadable. A date in the future (clock drift)
// counts as just now.
export function relativeTime(iso: string | undefined, now: Date): string {
  const t = Date.parse(iso ?? '')
  if (Number.isNaN(t)) return '—'
  const d = new Date(t)
  const minutes = Math.floor((now.getTime() - t) / 60_000)
  if (minutes < 1) return 'Just now'
  if (minutes < 60) return `${minutes} min ago`
  const hours = Math.floor(minutes / 60)
  const diff = dayDiff(d, now)
  if (hours < 6 && diff === 0) return `${hours} h ago`
  const clock = `${pad(d.getHours())}:${pad(d.getMinutes())}`
  if (diff === 0) return `Today, ${clock}`
  if (diff === 1) return `Yesterday, ${clock}`
  return `${dateLabel(d, now)}, ${clock}`
}

export function shortId(id: string): string {
  return id.length > 8 ? `${id.slice(0, 8)}…` : id
}

// The last two segments of a path, with "…/" in front when something was cut off: the end of a folder is
// the part that tells rows apart. Works for / and \ and ignores a trailing separator.
export function shortFolder(path: string): string {
  const parts = path.split(/[\\/]+/).filter((p) => p !== '')
  if (parts.length <= 2) return parts.join('/') === '' ? path : parts.join('/')
  return `…/${parts.slice(-2).join('/')}`
}

// What produced the log, read from the conversation title's fixed prefix. A reading aid only: an
// unknown title simply gets no chip.
const KINDS: [prefix: string, label: string][] = [
  ['generate job detail crawl instructions', 'Job details'],
  ['generate crawl instructions', 'Crawl instructions'],
  ['generate cv', 'CV'],
]

export function traceKind(title: string | undefined): string | null {
  const t = (title ?? '').trim().toLowerCase()
  return KINDS.find(([prefix]) => t.startsWith(prefix))?.[1] ?? null
}

export function pluralFiles(count: number): string {
  return count === 1 ? '1 file' : `${count} files`
}

export function totalFiles(entries: AllAiTraceLogsEntry[]): number {
  return entries.reduce((sum, e) => sum + e.logCount, 0)
}

// The most recent usable modifiedAt, or undefined when there is none.
export function latestModified(entries: AllAiTraceLogsEntry[]): string | undefined {
  let best: AllAiTraceLogsEntry | undefined
  for (const e of entries) {
    if (Number.isNaN(time(e))) continue
    if (!best || time(e) > time(best)) best = e
  }
  return best?.modifiedAt
}
