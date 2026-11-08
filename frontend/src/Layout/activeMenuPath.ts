import type { MenuEntry } from '../config/menu/menu'

// activeMenuPath (plan 200): which menu path is the current one. An entry whose path is a parent of the URL ("/tools/career/settings" of
// "/tools/career/settings/cleanup") matches too, because a detail page keeps its list entry current ("Platforms" on a platform's page); but
// when several entries match, only the LONGEST path is current, so a sibling that is nested under another entry's path ("Job cleanup" under
// "AI") is the only one highlighted. The pathname only: a query string never matters. null when no entry matches. Entries with the same
// path are all current (the result is a path, not an entry).
export function activeMenuPath(entries: MenuEntry[], pathname: string): string | null {
  let best: string | null = null
  const visit = (list: MenuEntry[]) => {
    for (const e of list) {
      if (e.path && (pathname === e.path || pathname.startsWith(e.path + '/')) && (best === null || e.path.length > best.length))
        best = e.path
      if (e.children) visit(e.children)
    }
  }
  visit(entries)
  return best
}
