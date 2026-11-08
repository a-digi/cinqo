import type { CheckFailure, ToolUpdate, UpdateCheck } from '../../../api/toolUpdates'
import { formatFetched } from './repositoryDisplay'

// Pure display and polling rules for the Tools page's update check, kept apart
// from React so they can be checked with a script. See
// plan/ai/tools/repository/check-for-updates.md.

// The page asks again every 1.5 s while a check runs, and gives up after 3 minutes.
export const POLL_INTERVAL_MS = 1500
export const POLL_LIMIT_MS = 3 * 60 * 1000

export function isRunning(check: Pick<UpdateCheck, 'state'>): boolean {
  return check.state === 'running'
}

// Keep asking only while the check runs and the limit has not passed.
export function shouldKeepPolling(check: Pick<UpdateCheck, 'state'>, elapsedMs: number): boolean {
  return isRunning(check) && elapsedMs < POLL_LIMIT_MS
}

export function gaveUpPolling(check: Pick<UpdateCheck, 'state'>, elapsedMs: number): boolean {
  return isRunning(check) && elapsedMs >= POLL_LIMIT_MS
}

export function buttonLabel(check: Pick<UpdateCheck, 'state'>): string {
  return isRunning(check) ? 'Checking…' : 'Check for updates'
}

// "Checking repositories… 2 of 4"
export function progressText(check: Pick<UpdateCheck, 'totalRepositories' | 'checkedRepositories'>): string {
  if (check.totalRepositories <= 0) return 'Checking repositories…'
  return `Checking repositories… ${Math.min(check.checkedRepositories, check.totalRepositories)} of ${check.totalRepositories}`
}

export function lastCheckedText(lastCheckedAt: string): string {
  return lastCheckedAt === '' ? 'Not checked yet' : `Last checked ${formatFetched(lastCheckedAt)}`
}

function plural(n: number, one: string, many: string): string {
  return n === 1 ? one : many
}

// Every repository failed: nothing can be said about updates, so the page must not
// claim that everything is up to date.
export function allFailed(check: Pick<UpdateCheck, 'totalRepositories' | 'failed'>): boolean {
  return check.totalRepositories > 0 && check.failed.length >= check.totalRepositories
}

export type BannerKind = 'update' | 'ok' | 'none'

// A banner shows when updates exist (from the stored catalogs, even before a check
// ran) or a check finished; "all up to date" only when it finished and not every
// repository failed.
export function bannerKind(check: Pick<UpdateCheck, 'state' | 'totalRepositories' | 'failed'>, updates: ToolUpdate[]): BannerKind {
  if (updates.length > 0) return 'update'
  if (check.state === 'done' && !allFailed(check)) return 'ok'
  return 'none'
}

export function bannerText(kind: BannerKind, updates: ToolUpdate[]): string {
  if (kind === 'update') return `${updates.length} ${plural(updates.length, 'update', 'updates')} available`
  if (kind === 'ok') return 'All installed tools are up to date'
  return ''
}

// "Could not check 1 repository: Cinqo tools"; empty when nothing failed.
export function failureText(failed: CheckFailure[]): string {
  if (failed.length === 0) return ''
  const names = failed.map((f) => f.name).join(', ')
  return `Could not check ${failed.length} ${plural(failed.length, 'repository', 'repositories')}: ${names}`
}

export function failureDetails(failed: CheckFailure[]): string {
  return failed.map((f) => `${f.name}: ${f.error}`).join('\n')
}

export function updateFor(updates: ToolUpdate[], slug: string): ToolUpdate | undefined {
  return updates.find((u) => u.slug === slug)
}

export function updateLabel(update: Pick<ToolUpdate, 'latestVersion'>): string {
  return `Update v${update.latestVersion}`
}

// What an update brings with it, in plain words: "Needs Browser updated first", "Also installs Browser and PDF Tools". Empty when it needs nothing.
export function needsText(update: Pick<ToolUpdate, 'needs'>): string {
  const list = (action: string) => update.needs.filter((n) => n.action === action).map((n) => n.name)
  const join = (names: string[]) => (names.length <= 1 ? names.join('') : `${names.slice(0, -1).join(', ')} and ${names[names.length - 1]}`)
  const parts: string[] = []
  if (list('update').length > 0) parts.push(`Needs ${join(list('update'))} updated first`)
  if (list('install').length > 0) parts.push(`Also installs ${join(list('install'))}`)
  if (list('enable').length > 0) parts.push(`Also enables ${join(list('enable'))}`)
  return parts.join('. ')
}
