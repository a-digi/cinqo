import { Link } from 'react-router-dom'
import { isValidBreadcrumbPath } from '../config/breadcrumbs/breadcrumbRules'
import { useBreadcrumbTrail } from '../config/breadcrumbs/useBreadcrumbTrail'
import { BackButton } from './BackButton'

// The bar at the top of the content area (plan/ai/frontend/step-270-breadcrumb.md): the GoBack button, then the trail registered
// for this page by the system or a tool. Nothing registered for the page = nothing drawn, not even the button.
// Labels are drawn as text and a link only when its path is a site-internal address (the items were cleaned already; this is the
// second check at the point of drawing).
export function Breadcrumbs() {
  const trail = useBreadcrumbTrail()
  if (trail.length === 0) return null
  return (
    <div className="flex shrink-0 items-center gap-2 border-b border-gray-200 bg-white px-3 py-1.5">
      <BackButton />
      <nav aria-label="Breadcrumb" className="min-w-0 flex-1 overflow-x-auto">
        <ol className="flex items-center whitespace-nowrap text-sm">
          {trail.map((item, i) => {
            const last = i === trail.length - 1
            return (
              <li key={i} className="flex min-w-0 items-center">
                {i > 0 && (
                  <span aria-hidden="true" className="px-2 text-gray-400">
                    ›
                  </span>
                )}
                {!last && item.path !== undefined && isValidBreadcrumbPath(item.path) ? (
                  <Link to={item.path} title={item.label} className="max-w-[16rem] truncate text-gray-500 hover:text-gray-900 hover:underline">
                    {item.label}
                  </Link>
                ) : (
                  <span
                    title={item.label}
                    aria-current={last ? 'page' : undefined}
                    className={`max-w-[16rem] truncate ${last ? 'font-medium text-gray-900' : 'text-gray-500'}`}
                  >
                    {item.label}
                  </span>
                )}
              </li>
            )
          })}
        </ol>
      </nav>
    </div>
  )
}
