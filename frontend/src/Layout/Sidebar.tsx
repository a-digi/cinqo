import { useMenuGroups } from '../config/menu/useMenuGroups'
import { SidebarMenuItem } from './SidebarMenuItem'

// Data-driven, recursively nestable — see config/menu/menu.ts and
// plan/ai/frontend/frontend/step-06-nested-menu-system.md. Replaces the
// previous hardcoded flat list.
export function Sidebar() {
  const groups = useMenuGroups()
  return (
    <nav className="w-56 shrink-0 border-r border-gray-200 bg-white p-4">
      <ul className="space-y-1">
        {groups.map((entry) => (
          <SidebarMenuItem key={entry.label} entry={entry} depth={1} />
        ))}
      </ul>
    </nav>
  )
}
