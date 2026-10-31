// Pure rules for matching a tool's page to the address in the browser (no React, so they can be script-checked).
//
// A tool registers a route with a path. It is either exact ("/tools/career/skills") or a pattern with named
// segments ("/tools/career/skills/:personaId"): a ":name" segment matches exactly one non-empty segment of the
// address and hands its (URL-decoded) text to the tool's page. An exact path always wins over a pattern; among
// patterns the first one registered that fits wins.

export interface PathRoute {
  path: string
}

export interface ToolRouteMatch<T extends PathRoute> {
  route: T
  // The text of each ":name" segment, by name; empty for an exact path.
  params: Record<string, string>
}

const PARAM_NAME = /^[A-Za-z][A-Za-z0-9_]*$/

// The name of a ":name" segment, or null for an ordinary one (also for a ":" with an unusable name).
function paramName(segment: string): string | null {
  if (!segment.startsWith(':')) return null
  const name = segment.slice(1)
  return PARAM_NAME.test(name) ? name : null
}

// Whether a tool may register this path: it starts with "/tools/", has no empty segment (no "//", no trailing
// "/"), and uses each ":name" once.
export function isValidToolRoutePath(path: string): boolean {
  if (!path.startsWith('/tools/')) return false
  const segments = path.split('/').slice(1)
  if (segments.some((s) => s === '')) return false
  const names = segments.map(paramName).filter((n): n is string => n !== null)
  return new Set(names).size === names.length
}

// The route for an address (a pathname), or null when none fits.
export function matchToolRoute<T extends PathRoute>(routes: readonly T[], pathname: string): ToolRouteMatch<T> | null {
  const exact = routes.find((r) => r.path === pathname)
  if (exact) return { route: exact, params: {} }

  const parts = pathname.split('/')
  for (const route of routes) {
    const pattern = route.path.split('/')
    if (pattern.length !== parts.length) continue
    const params: Record<string, string> = {}
    let fits = true
    for (let i = 0; i < pattern.length && fits; i++) {
      const name = paramName(pattern[i])
      if (name === null) {
        fits = pattern[i] === parts[i]
        continue
      }
      let value: string
      try {
        value = decodeURIComponent(parts[i])
      } catch {
        fits = false
        continue
      }
      if (value === '') fits = false
      else params[name] = value
    }
    if (fits) return { route, params }
  }
  return null
}
