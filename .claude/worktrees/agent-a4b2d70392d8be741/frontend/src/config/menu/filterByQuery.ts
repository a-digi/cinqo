import type { MenuEntry } from './menu'

// Recursive, same shape as filterVisible.ts: an entry survives if its
// own label matches OR at least one descendant survives. Unlike
// filterVisible (which prunes a parent's children down to only the
// visible ones), a parent whose OWN label matches keeps every one of
// its children as-is — searching "career" should show the whole Career
// group, not just whichever of its children also happen to contain
// "career". Only a non-matching parent gets pruned down to its
// matching descendants. Case-insensitive substring match, matching how
// a user actually types a search term (no fuzzy matching, no need for
// one over a menu this size).
export function filterByQuery(entries: MenuEntry[], query: string): MenuEntry[] {
  const q = query.toLowerCase()
  const result: MenuEntry[] = []
  for (const entry of entries) {
    const ownMatch = entry.label.toLowerCase().includes(q)
    if (ownMatch) {
      result.push(entry)
      continue
    }
    if (entry.children) {
      const filteredChildren = filterByQuery(entry.children, query)
      if (filteredChildren.length > 0) {
        result.push({ ...entry, children: filteredChildren })
      }
    }
  }
  return result
}
