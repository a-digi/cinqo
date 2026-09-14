import { useState } from 'react'
import { NavLink, useLocation } from 'react-router-dom'
import type { MenuEntry } from '../config/menu/menu'
import { SidebarIcon } from './SidebarIcon'

export function hasActiveDescendant(entry: MenuEntry, pathname: string): boolean {
  if (entry.path && (pathname === entry.path || pathname.startsWith(entry.path + '/'))) return true
  return entry.children?.some((child) => hasActiveDescendant(child, pathname)) ?? false
}

// Calculated, not looked up — depth * step reproduces the old fixed
// pl-3/pl-6/pl-9 progression exactly for depths 1-3 (Tailwind's own
// N*4px spacing scale: 12px/24px/36px = depth*12px) but, unlike a
// small fixed lookup table, scales correctly to any depth instead of
// capping. A plain inline style, not a Tailwind class: Tailwind only
// ever emits CSS for class names its build-time scanner can see
// literally in source, so a class built from a runtime `depth`
// variable would never have a matching compiled rule. See
// plan/ai/frontend/frontend/step-13-sidebar-icons-and-calculated-indentation.md.
const SIDEBAR_INDENT_STEP_PX = 12

// Recursive: one component renders every depth. A path-less entry
// (System, Security) is a plain toggle button, not a link — clicking it
// expands/collapses its children instead of navigating anywhere. See
// plan/ai/frontend/frontend/step-06-nested-menu-system.md.
export function SidebarMenuItem({ entry, depth }: { entry: MenuEntry; depth: number }) {
  const location = useLocation()
  const [isExpanded, setIsExpanded] = useState(() => hasActiveDescendant(entry, location.pathname))
  const hasChildren = !!entry.children?.length
  const paddingLeft = depth * SIDEBAR_INDENT_STEP_PX

  // An icon-less entry still reserves the same icon-slot width as its
  // siblings, so labels at the same depth line up whether or not each
  // one has its own icon.
  const iconSlot = entry.icon ? <SidebarIcon svg={entry.icon} /> : <span className="inline-block h-4 w-4 shrink-0" />

  return (
    <li>
      <div className="flex items-center">
        {entry.path ? (
          <NavLink
            to={entry.path}
            end={entry.path === '/'}
            style={{ paddingLeft }}
            className={({ isActive }) =>
              `flex flex-1 items-center gap-2 rounded-md py-2 pr-3 text-sm ${
                isActive ? 'bg-gray-100 font-medium text-gray-900' : 'text-gray-600 hover:bg-gray-50'
              }`
            }
          >
            {iconSlot}
            {entry.label}
          </NavLink>
        ) : (
          <button
            type="button"
            onClick={() => setIsExpanded((prev) => !prev)}
            aria-expanded={isExpanded}
            style={{ paddingLeft }}
            className="flex flex-1 items-center gap-2 rounded-md py-2 pr-3 text-left text-sm font-medium text-gray-500"
          >
            {iconSlot}
            {entry.label}
          </button>
        )}
        {hasChildren && (
          <button
            type="button"
            onClick={() => setIsExpanded((prev) => !prev)}
            aria-expanded={isExpanded}
            aria-label={isExpanded ? `Collapse ${entry.label}` : `Expand ${entry.label}`}
            className="px-2 py-2 text-gray-400 hover:text-gray-600"
          >
            <CaretIcon open={isExpanded} />
          </button>
        )}
      </div>
      {hasChildren && isExpanded && (
        <ul>
          {entry.children!.map((child) => (
            <SidebarMenuItem key={child.label} entry={child} depth={depth + 1} />
          ))}
        </ul>
      )}
    </li>
  )
}

function CaretIcon({ open }: { open: boolean }) {
  return (
    <svg
      viewBox="0 0 20 20"
      fill="currentColor"
      className={`h-4 w-4 transition-transform ${open ? 'rotate-90' : ''}`}
    >
      <path fillRule="evenodd" d="M7.293 4.293a1 1 0 0 1 1.414 0l5 5a1 1 0 0 1 0 1.414l-5 5a1 1 0 0 1-1.414-1.414L11.586 10 7.293 5.707a1 1 0 0 1 0-1.414Z" clipRule="evenodd" />
    </svg>
  )
}
