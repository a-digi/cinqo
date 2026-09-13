import { useAuth } from '../Components/Auth/AuthContext'

// Minimal top bar. onToggleSidebar shows/hides Layout's <Sidebar /> —
// see plan/ai/frontend/frontend/step-08-collapsible-sidebars.md.
export function Navbar({ onToggleSidebar }: { onToggleSidebar: () => void }) {
  const { logout } = useAuth()
  return (
    <header className="flex h-14 shrink-0 items-center justify-between border-b border-gray-200 bg-white px-4">
      <div className="flex items-center gap-2">
        <button
          type="button"
          onClick={onToggleSidebar}
          aria-label="Toggle sidebar"
          className="rounded p-1.5 text-gray-500 hover:bg-gray-100"
        >
          <MenuIcon />
        </button>
        <span className="text-lg font-semibold text-gray-900">cinqo</span>
      </div>
      <button
        type="button"
        onClick={() => void logout()}
        className="rounded-md border border-gray-300 px-3 py-1 text-sm text-gray-700 hover:bg-gray-50"
      >
        Sign out
      </button>
    </header>
  )
}

function MenuIcon() {
  return (
    <svg viewBox="0 0 20 20" fill="currentColor" className="h-5 w-5">
      <path fillRule="evenodd" d="M2 5.5A.5.5 0 0 1 2.5 5h15a.5.5 0 0 1 0 1h-15a.5.5 0 0 1-.5-.5Zm0 4.5a.5.5 0 0 1 .5-.5h15a.5.5 0 0 1 0 1h-15A.5.5 0 0 1 2 10Zm0 4.5a.5.5 0 0 1 .5-.5h15a.5.5 0 0 1 0 1h-15a.5.5 0 0 1-.5-.5Z" clipRule="evenodd" />
    </svg>
  )
}
