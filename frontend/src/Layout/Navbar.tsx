import { useAuth } from '../Components/Auth/AuthContext'

// Minimal top bar — no menu-config system yet (add one only once there
// are enough real pages to need a data-driven menu).
export function Navbar() {
  const { logout } = useAuth()
  return (
    <header className="flex h-14 shrink-0 items-center justify-between border-b border-gray-200 bg-white px-4">
      <span className="text-lg font-semibold text-gray-900">cinqo</span>
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
