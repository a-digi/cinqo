import { Outlet } from 'react-router-dom'

export function Content() {
  return (
    <main className="min-w-0 flex-1 overflow-y-auto p-6">
      <Outlet />
    </main>
  )
}
