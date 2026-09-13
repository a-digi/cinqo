import { useState } from 'react'
import { NavLink, useLocation } from 'react-router-dom'
import type { MenuEntry } from '../config/menu/menu'

export function hasActiveDescendant(entry: MenuEntry, pathname: string): boolean {
  if (entry.path && (pathname === entry.path || pathname.startsWith(entry.path + '/'))) return true
  return entry.children?.some((child) => hasActiveDescendant(child, pathname)) ?? false
}

const INDENT_CLASSES = ['pl-3', 'pl-6', 'pl-9']

// Recursive: one component renders every depth. A path-less entry
// (System, Security) is a plain toggle button, not a link — clicking it
// expands/collapses its children instead of navigating anywhere. See
// plan/ai/frontend/frontend/step-06-nested-menu-system.md.
export function SidebarMenuItem({ entry, depth }: { entry: MenuEntry; depth: number }) {
  const location = useLocation()
  const [isExpanded, setIsExpanded] = useState(() => hasActiveDescendant(entry, location.pathname))
  const hasChildren = !!entry.children?.length
  const indent = INDENT_CLASSES[Math.min(depth - 1, INDENT_CLASSES.length - 1)]

  return (
    <li>
      <div className="flex items-center">
        {entry.path ? (
          <NavLink
            to={entry.path}
            end={entry.path === '/'}
            className={({ isActive }) =>
              `flex-1 rounded-md px-3 py-2 text-sm ${indent} ${
                isActive ? 'bg-gray-100 font-medium text-gray-900' : 'text-gray-600 hover:bg-gray-50'
              }`
            }
          >
            {entry.label}
          </NavLink>
        ) : (
          <button
            type="button"
            onClick={() => setIsExpanded((prev) => !prev)}
            aria-expanded={isExpanded}
            className={`flex-1 rounded-md px-3 py-2 text-left text-sm font-medium text-gray-500 ${indent}`}
          >
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
