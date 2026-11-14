// Pure rules for the breadcrumb bar (no React, so they can be script-checked). plan/ai/frontend/step-270-breadcrumb.md.
//
// The system and the tools register a trail for a route: a path (exact, or with ":name" segments, matched exactly like a
// tool route) and the items to show. On each page load the trail for the address is looked up; no registration for it
// means no trail, and then nothing is drawn.

import { matchToolRoute } from '../tools/matchToolRoute'

export interface BreadcrumbItem {
  // Plain text, 1 to MAX_LABEL characters.
  label: string
  // A site-internal address. Without one the item is plain text. The last item is always the current page, never a link.
  path?: string
}

// What a trail function gets: the text of each ":name" segment of the address, and its query string ("?a=b" or "").
export interface BreadcrumbContext {
  params: Record<string, string>
  search: string
}

export type BreadcrumbItems = BreadcrumbItem[] | ((context: BreadcrumbContext) => BreadcrumbItem[])

export interface BreadcrumbRegistration {
  path: string
  items: BreadcrumbItems
}

export const MAX_ITEMS = 8
export const MAX_LABEL = 60
export const MAX_PATH = 200

const PARAM_NAME = /^[A-Za-z][A-Za-z0-9_]*$/

// A link target is allowed only when it stays on this site: it starts with one "/" (so no "//host", no "http:", no
// "javascript:"), has no backslash (some browsers read "/\host" as "//host") and no control character or blank.
export function isValidBreadcrumbPath(path: unknown): path is string {
  if (typeof path !== 'string') return false
  if (path.length > MAX_PATH) return false
  if (!path.startsWith('/') || path.startsWith('//')) return false
  // eslint-disable-next-line no-control-regex
  return !/[\\\u0000- \u007f]/.test(path)
}

// Whether a path may be registered as a trail's route: "/" or segments that are not empty (no "//", no trailing "/"),
// each ":name" used once. The same shape rule a tool route has, without its "/tools/" prefix (the system registers any
// path of the shell; for a tool the prefix is checked on top, see isValidToolRoutePath).
export function isValidRegistrationPath(path: unknown): path is string {
  if (typeof path !== 'string' || path.length > MAX_PATH) return false
  if (path === '/') return true
  if (!path.startsWith('/')) return false
  const segments = path.split('/').slice(1)
  if (segments.some((s) => s === '')) return false
  const names: string[] = []
  for (const s of segments) {
    if (!s.startsWith(':')) continue
    const name = s.slice(1)
    if (!PARAM_NAME.test(name)) return false
    names.push(name)
  }
  return new Set(names).size === names.length
}

// What may be drawn from whatever a source handed over. An item that is not an object, has no text, or has a path that
// is not a site-internal address is dropped; a label is trimmed and cut to MAX_LABEL; when more than MAX_ITEMS remain the
// LAST MAX_ITEMS are kept (the end of the trail is the page the person is on). Returns new objects, never the input's.
export function cleanItems(input: unknown): BreadcrumbItem[] {
  if (!Array.isArray(input)) return []
  const out: BreadcrumbItem[] = []
  for (const raw of input as unknown[]) {
    if (typeof raw !== 'object' || raw === null) continue
    const { label, path } = raw as { label?: unknown; path?: unknown }
    if (typeof label !== 'string') continue
    const text = label.trim().slice(0, MAX_LABEL).trim()
    if (text === '') continue
    if (path === undefined) {
      out.push({ label: text })
      continue
    }
    if (!isValidBreadcrumbPath(path)) continue
    out.push({ label: text, path })
  }
  return out.slice(-MAX_ITEMS)
}

// The trail for an address: the registration that fits (an exact path beats a pattern, the first registered pattern
// wins), its items made by the function when it is one, then cleaned. Nothing fits, or the function throws: no trail.
export function resolveBreadcrumbs(
  registrations: readonly BreadcrumbRegistration[],
  pathname: string,
  search: string,
): BreadcrumbItem[] {
  const match = matchToolRoute(registrations, pathname)
  if (!match) return []
  const { route, params } = match
  if (typeof route.items !== 'function') return cleanItems(route.items)
  try {
    return cleanItems(route.items({ params, search }))
  } catch (error) {
    console.error(`Breadcrumb trail for "${route.path}" failed — no trail shown.`, error)
    return []
  }
}

// Whether there is an earlier page of this app to go back to. React Router keeps an index in history.state: 0 is the
// first page this tab showed (opened directly or in a new tab), where going back would leave the app.
export function canGoBack(historyState: unknown): boolean {
  if (typeof historyState !== 'object' || historyState === null) return false
  const idx = (historyState as { idx?: unknown }).idx
  return Number.isInteger(idx) && (idx as number) > 0
}

// Adds a registration to the list. Refused (list unchanged, `reason` says why) when the path or the items are not usable, or when
// the same path is already registered: the first registration of a path wins. Never mutates the list it is given.
export function addRegistration(
  list: readonly BreadcrumbRegistration[],
  registration: BreadcrumbRegistration,
): { list: readonly BreadcrumbRegistration[]; reason: string | null } {
  const given: unknown = registration
  if (typeof given !== 'object' || given === null) return { list, reason: 'a registration is needed' }
  if (!isValidRegistrationPath(registration.path)) {
    return { list, reason: `"${String(registration.path)}" is not a usable path (segments must not be empty, each ":name" once)` }
  }
  if (!Array.isArray(registration.items) && typeof registration.items !== 'function') {
    return { list, reason: 'items must be an array or a function that returns one' }
  }
  if (list.some((r) => r.path === registration.path)) {
    return { list, reason: `"${registration.path}" already has a trail (the first registration wins)` }
  }
  return { list: [...list, { path: registration.path, items: registration.items }], reason: null }
}
