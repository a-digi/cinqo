import { Outlet } from 'react-router-dom'
import { Breadcrumbs } from './Breadcrumbs'

// The breadcrumb bar sits ABOVE the scrolling main (plan/ai/frontend/step-270-breadcrumb.md), so it never scrolls away and a page's
// own sticky header is not affected. With no trail registered for the page the bar draws nothing and main fills the column as before.
export function Content() {
  return (
    <div className="flex min-h-0 min-w-0 flex-1 flex-col">
      <Breadcrumbs />
      <main className="min-h-0 min-w-0 flex-1 overflow-y-auto p-0">
        <Outlet />
      </main>
    </div>
  )
}
