import { AppScopes } from './scopes'

// Every scope string this frontend references, anywhere — the full
// AppScopes catalog, not just the subset menu.ts currently wires up.
// Used to catch a scope that's gone stale (renamed/removed
// backend-side) against the backend's registered catalog — see
// plan/ai/security/frontend-scope-conformance.md.
export function referencedScopeIds(): string[] {
  return Object.values(AppScopes)
}
