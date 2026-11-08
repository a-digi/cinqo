// Job crawler settings (step 150): the pure rules and words of the Settings → Job crawler page ("Ignore jobs"), kept apart from the
// component so they can be checked on their own. The bounds are the backend's (crawlcriteria/settings.go and the schema CHECKs).
import { deletedAtText } from './deletedJobsRules'

export const MATCH_PERCENT = { min: 1, max: 100 }

// What GET /settings/crawler answers and PUT takes.
export interface CrawlerSettings {
  ignoreEnabled: boolean
  minMatchEnabled: boolean
  minMatchPercent: number
  updatedAt: string
}

// What a save sends: updatedAt is the server's.
export type EditableCrawlerSettings = Omit<CrawlerSettings, 'updatedAt'>

// One row of the log of jobs the rules kept out (GET /settings/crawler/ignored).
export interface IgnoredJob {
  id: number
  sourceUrl: string
  title: string
  company: string
  location: string
  portalName: string
  matcher: string
  reason: string
  ignoredAt: string
}

export interface IgnoredList {
  items: IgnoredJob[]
  total: number
}

// The form keeps the number as the text typed, so an empty or half-typed box is an error, not a silent 0.
export interface CrawlerForm {
  ignoreEnabled: boolean
  minMatchEnabled: boolean
  minMatchPercent: string
}

export function toForm(s: CrawlerSettings): CrawlerForm {
  return { ignoreEnabled: s.ignoreEnabled, minMatchEnabled: s.minMatchEnabled, minMatchPercent: String(s.minMatchPercent) }
}

function wholeNumber(text: string, range: { min: number; max: number }): number | null {
  const t = text.trim()
  if (!/^\d+$/.test(t)) return null
  const n = Number(t)
  return n >= range.min && n <= range.max ? n : null
}

export interface FormErrors {
  minMatchPercent?: string
}

// The limit is checked even while its rule is off, because the server stores (and checks) it either way.
export function formErrors(f: CrawlerForm): FormErrors {
  const e: FormErrors = {}
  if (wholeNumber(f.minMatchPercent, MATCH_PERCENT) === null) {
    e.minMatchPercent = `A whole percentage from ${String(MATCH_PERCENT.min)} to ${String(MATCH_PERCENT.max)}.`
  }
  return e
}

export function isValid(f: CrawlerForm): boolean {
  return Object.keys(formErrors(f)).length === 0
}

// The settings to save, or null while the form has an error.
export function toSettings(f: CrawlerForm): EditableCrawlerSettings | null {
  const percent = wholeNumber(f.minMatchPercent, MATCH_PERCENT)
  if (percent === null) return null
  return { ignoreEnabled: f.ignoreEnabled, minMatchEnabled: f.minMatchEnabled, minMatchPercent: percent }
}

// Unsaved changes: the form differs from what is stored (a half-typed number counts as a change).
export function isChanged(f: CrawlerForm, saved: CrawlerSettings): boolean {
  return JSON.stringify(f) !== JSON.stringify(toForm(saved))
}

// What the settings do, in plain words (for the saved settings, and again after a save).
export function summaryLine(s: EditableCrawlerSettings): string {
  if (!s.ignoreEnabled) return 'Ignore jobs is off: the crawler inserts every job it finds.'
  if (!s.minMatchEnabled) return 'Ignore jobs is on, but no rule is switched on: the crawler inserts every job it finds.'
  return `The crawler inserts a new job only if at least one profile matches it with ${String(s.minMatchPercent)}% or more (a job no profile can judge is inserted too).`
}

// What the page says about the risk of judging a job from its listing.
export const RISK_NOTE =
  'The match is made from what the job list shows (title, company, place, a short text), so it can be lower than the match the full posting would give. ' +
  'A job kept out here is looked at again at the next crawl, so changing a rule or a profile can let it in later. Jobs that are already saved are never removed or hidden by these rules.'

export function savedNotice(s: EditableCrawlerSettings): string {
  return `Saved. ${summaryLine(s)}`
}

// The message when saving failed: the server's words, with its trailing punctuation tidied.
export function saveFailureText(message: string): string {
  return `Could not save the settings: ${message.replace(/[.\s]+$/, '')}.`
}

// ---- the log of ignored jobs ----

// The name of a rule as the page shows it; a rule this version does not know is shown by its own name.
export function matcherLabel(name: string): string {
  return name === 'min-profile-match' ? 'Minimum profile match' : name
}

// The line above the list.
export function ignoredHeadline(l: IgnoredList): string {
  if (l.total === 0) return 'No job has been kept out yet.'
  const base = l.total === 1 ? '1 job was kept out by your rules.' : `${String(l.total)} jobs were kept out by your rules.`
  return l.items.length < l.total ? `${base} Showing the newest ${String(l.items.length)}.` : base
}

// What the empty list says, depending on whether the feature is on.
export function ignoredEmptyText(ignoreEnabled: boolean): string {
  return ignoreEnabled ? 'No job has been kept out yet.' : 'Nothing is kept out while Ignore jobs is off.'
}

// What a row says about a job: company, place and portal where known, joined with a middle dot.
export function ignoredDetail(j: IgnoredJob): string {
  return [j.company, j.location, j.portalName].filter((p) => p !== '').join(' · ')
}

// Why a job was kept out: the rule and its reason, e.g. "Minimum profile match: Best profile match 0% is under the limit of 30%".
export function ignoredReason(j: IgnoredJob): string {
  const reason = j.reason === '' ? '' : j.reason.charAt(0).toUpperCase() + j.reason.slice(1)
  return reason === '' ? matcherLabel(j.matcher) : `${matcherLabel(j.matcher)}: ${reason}`
}

// When it was kept out, in the viewer's local time.
export function ignoredWhen(j: IgnoredJob): string {
  return deletedAtText(j.ignoredAt)
}
