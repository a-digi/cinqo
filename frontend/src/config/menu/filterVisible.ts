import type { MenuEntry } from './menu'

// Recursive: a parent is never scope-checked directly — it survives
// only if at least one filtered child survives. A leaf is visible if
// the user is super-admin, declares no scopes (always visible), or the
// user's scopes intersect its scopes (OR, not AND). Adapted verbatim
// from coco-mda's real, working menu filter — see
// plan/ai/frontend/frontend/step-06-nested-menu-system.md.
export function filterVisible(entries: MenuEntry[], isSuperAdmin: boolean, userScopes: string[]): MenuEntry[] {
  const result: MenuEntry[] = []
  for (const entry of entries) {
    if (entry.children) {
      const visibleChildren = filterVisible(entry.children, isSuperAdmin, userScopes)
      if (visibleChildren.length > 0) {
        result.push({ ...entry, children: visibleChildren })
      }
      continue
    }
    const isVisible = isSuperAdmin || !entry.scopes || entry.scopes.some((scope) => userScopes.includes(scope))
    if (isVisible) result.push(entry)
  }
  return result
}
