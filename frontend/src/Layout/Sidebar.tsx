import { useMemo, useState } from 'react'
import { useMenuGroups } from '../config/menu/useMenuGroups'
import { filterByQuery } from '../config/menu/filterByQuery'
import { SidebarMenuItem } from './SidebarMenuItem'

// Data-driven, recursively nestable — see config/menu/menu.ts and
// plan/ai/frontend/frontend/step-06-nested-menu-system.md. Replaces the
// previous hardcoded flat list.
//
// The search bar and the scrollable list are two separate flex
// children of this <nav> (shrink-0 vs flex-1 overflow-y-auto) — the
// search bar never scrolls out of view, and the list scrolls within
// whatever height Layout.tsx's own flex-1 overflow-hidden row already
// leaves for the sidebar, instead of silently being clipped by it the
// way the previous single, non-scrolling <ul> was. See
// plan/ai/frontend/frontend/step-XX-sidebar-search-and-scroll.md.
export function Sidebar() {
  const groups = useMenuGroups()
  const [query, setQuery] = useState('')
  const filteredGroups = useMemo(() => (query.trim() ? filterByQuery(groups, query.trim()) : groups), [groups, query])

  return (
    <nav className="flex h-full w-56 shrink-0 flex-col border-r border-gray-200 bg-white">
      <div className="shrink-0 border-b border-gray-200 p-3">
        <input
          type="text"
          value={query}
          onChange={(e) => {
            setQuery(e.target.value)
          }}
          placeholder="Search menu…"
          className="w-full rounded-md border border-gray-200 px-2 py-1.5 text-sm focus:border-gray-900 focus:outline-none focus:ring-1 focus:ring-gray-900"
        />
      </div>
      <div className="flex-1 overflow-y-auto p-4">
        <ul className="space-y-1">
          {filteredGroups.map((entry) => (
            <SidebarMenuItem key={entry.label} entry={entry} depth={1} forceExpanded={query.trim().length > 0} />
          ))}
        </ul>
      </div>
    </nav>
  )
}
