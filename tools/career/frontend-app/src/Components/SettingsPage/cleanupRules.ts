// Job cleanup (step 131): the pure rules of the Settings → Job cleanup page, kept apart from the component so they
// can be checked on their own. The bounds are the backend's (settings.go and the schema CHECKs).
import type { CleanupPreview, CleanupRequest, CleanupRule, CleanupSettings } from '../../api'

export const OLDER_DAYS = { min: 1, max: 3650 }
export const MATCH_PERCENT = { min: 1, max: 100 }
export const INTERVAL_HOURS = { min: 1, max: 168 }

// The form keeps the numbers as the text typed, so an empty or half-typed box is an error, not a silent 0.
export interface CleanupForm {
  enabled: boolean
  olderEnabled: boolean
  olderThanDays: string
  lowMatchEnabled: boolean
  minMatchPercent: string
  includeManual: boolean
  intervalHours: string
}

export type EditableSettings = Omit<CleanupSettings, 'lastRunAt' | 'lastRunDeleted' | 'lastRunSummary'>

export function toForm(s: CleanupSettings): CleanupForm {
  return {
    enabled: s.enabled,
    olderEnabled: s.olderEnabled,
    olderThanDays: String(s.olderThanDays),
    lowMatchEnabled: s.lowMatchEnabled,
    minMatchPercent: String(s.minMatchPercent),
    includeManual: s.includeManual,
    intervalHours: String(s.intervalHours),
  }
}

function wholeNumber(text: string, range: { min: number; max: number }): number | null {
  const t = text.trim()
  if (!/^\d+$/.test(t)) return null
  const n = Number(t)
  return n >= range.min && n <= range.max ? n : null
}

export interface FormErrors {
  olderThanDays?: string
  minMatchPercent?: string
  intervalHours?: string
}

export function formErrors(f: CleanupForm): FormErrors {
  const e: FormErrors = {}
  if (wholeNumber(f.olderThanDays, OLDER_DAYS) === null) e.olderThanDays = `A whole number of days from ${String(OLDER_DAYS.min)} to ${String(OLDER_DAYS.max)}.`
  if (wholeNumber(f.minMatchPercent, MATCH_PERCENT) === null) e.minMatchPercent = `A whole percentage from ${String(MATCH_PERCENT.min)} to ${String(MATCH_PERCENT.max)}.`
  if (wholeNumber(f.intervalHours, INTERVAL_HOURS) === null) e.intervalHours = `A whole number of hours from ${String(INTERVAL_HOURS.min)} to ${String(INTERVAL_HOURS.max)}.`
  return e
}

export function isValid(f: CleanupForm): boolean {
  return Object.keys(formErrors(f)).length === 0
}

// The settings to save, or null while the form has an error.
export function toSettings(f: CleanupForm): EditableSettings | null {
  const days = wholeNumber(f.olderThanDays, OLDER_DAYS)
  const percent = wholeNumber(f.minMatchPercent, MATCH_PERCENT)
  const hours = wholeNumber(f.intervalHours, INTERVAL_HOURS)
  if (days === null || percent === null || hours === null) return null
  return {
    enabled: f.enabled,
    olderEnabled: f.olderEnabled,
    olderThanDays: days,
    lowMatchEnabled: f.lowMatchEnabled,
    minMatchPercent: percent,
    includeManual: f.includeManual,
    intervalHours: hours,
  }
}

// Unsaved changes: the form differs from what is stored.
export function isChanged(f: CleanupForm, saved: CleanupSettings): boolean {
  return JSON.stringify(f) !== JSON.stringify(toForm(saved))
}

// Which rule a preview or a run uses: the one(s) switched on in the form. null when none is on.
export function previewRule(f: CleanupForm): CleanupRule | null {
  if (f.olderEnabled && f.lowMatchEnabled) return 'both'
  if (f.olderEnabled) return 'older'
  if (f.lowMatchEnabled) return 'low_match'
  return null
}

// What a preview or a run sends: the numbers typed in the form, saved or not, so the preview is of this form. null
// while no rule is on or a number is invalid.
export function cleanupRequest(f: CleanupForm): CleanupRequest | null {
  const rule = previewRule(f)
  const s = toSettings(f)
  if (rule === null || s === null) return null
  return { rule, olderThanDays: s.olderThanDays, minMatchPercent: s.minMatchPercent, includeManual: s.includeManual }
}

// A preview belongs to the exact request that made it; any change to the form makes it stale, and a stale preview
// can never be deleted from.
export function requestKey(r: CleanupRequest | null): string {
  return r === null ? '' : JSON.stringify(r)
}

// A warning when a run would remove most of the jobs (more than half of all of them).
export function largeRunWarning(p: CleanupPreview): string {
  if (p.totalJobs === 0 || p.wouldDelete * 2 <= p.totalJobs) return ''
  return `This would delete ${String(p.wouldDelete)} of ${String(p.totalJobs)} jobs (${String(Math.round((p.wouldDelete * 100) / p.totalJobs))} %). Check the limits before you go on.`
}

function plural(n: number, one: string, many: string): string {
  return `${String(n)} ${n === 1 ? one : many}`
}

// Why jobs a rule picked are still kept, one line per reason that applies (nothing for a reason that does not).
export function keptLines(p: CleanupPreview): string[] {
  const k = p.kept
  const lines: string[] = []
  if (k.onBoard > 0) lines.push(`${plural(k.onBoard, 'job is', 'jobs are')} kept because ${k.onBoard === 1 ? 'it is' : 'they are'} on a job board`)
  if (k.hasCv > 0) lines.push(`${plural(k.hasCv, 'job is', 'jobs are')} kept because ${k.hasCv === 1 ? 'it has' : 'they have'} a CV`)
  if (k.inProgress > 0) lines.push(`${plural(k.inProgress, 'job is', 'jobs are')} kept because a match is running`)
  if (k.manual > 0) lines.push(`${plural(k.manual, 'job is', 'jobs are')} kept because you added ${k.manual === 1 ? 'it' : 'them'} by hand`)
  if (k.unjudged > 0) lines.push(`${plural(k.unjudged, 'job is', 'jobs are')} kept because no profile could judge ${k.unjudged === 1 ? 'it' : 'them'}`)
  return lines
}

// The headline of a preview.
export function previewHeadline(p: CleanupPreview): string {
  if (p.wouldDelete === 0) return 'Nothing would be deleted.'
  const parts: string[] = []
  if (p.byRule.older > 0) parts.push(`${String(p.byRule.older)} too old`)
  if (p.byRule.lowMatch > 0) parts.push(`${String(p.byRule.lowMatch)} matching no profile well`)
  return `${plural(p.wouldDelete, 'job would be', 'jobs would be')} deleted (${parts.join(', ')}).`
}

// "2026-06-30 12:00:00" (UTC, what the server stores) as the viewer's local date and time; '' when never.
export function lastRunText(s: CleanupSettings): string {
  if (s.lastRunAt === '') return 'Never run.'
  const date = new Date(`${s.lastRunAt.replace(' ', 'T')}Z`)
  const when = Number.isNaN(date.getTime()) ? s.lastRunAt : date.toLocaleString()
  return `Last run ${when}: ${s.lastRunSummary === '' ? `${String(s.lastRunDeleted)} deleted` : s.lastRunSummary}.`
}

// What the automatic run will do, in words, for the switch's hint; '' when it is off.
export function automaticText(f: CleanupForm): string {
  if (!f.enabled) return ''
  const rule = previewRule(f)
  if (rule === null) return 'The automatic run is on, but no rule is switched on, so it will do nothing.'
  const hours = wholeNumber(f.intervalHours, INTERVAL_HOURS)
  if (hours === null) return ''
  return `Runs every ${plural(hours, 'hour', 'hours')}; the first run comes ${plural(hours, 'hour', 'hours')} after you save, so there is time to change your mind.`
}
