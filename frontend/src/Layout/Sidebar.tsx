import { NavLink } from 'react-router-dom'

// A hardcoded nav list — no menu-config system yet (coco-mda's
// config/menu/menu.ts + useMenuGroups). Add one only once there are
// enough real pages to need a data-driven menu; a hardcoded list is fine
// until then.
export function Sidebar() {
  return (
    <nav className="w-56 shrink-0 border-r border-gray-200 bg-white p-4">
      <ul className="space-y-1">
        <li>
          <NavLink
            to="/"
            end
            className={({ isActive }) =>
              `block rounded-md px-3 py-2 text-sm ${
                isActive ? 'bg-gray-100 font-medium text-gray-900' : 'text-gray-600 hover:bg-gray-50'
              }`
            }
          >
            Home
          </NavLink>
        </li>
      </ul>
    </nav>
  )
}
