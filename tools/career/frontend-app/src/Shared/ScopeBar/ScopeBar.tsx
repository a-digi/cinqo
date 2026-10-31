import { PAGE_ACCENT } from '../../Components/JobsPage/jobsTheme'

// The bar above a page's content that says whose data the page shows (the profile or
// persona picked in a switcher), with a "Manage" link to where that list is kept. It is
// a filter on the page, so it sits next to the content, not inside the header card. A page
// can put its main action ("New CV") in the bar, through `actions`: it sits at the right edge of the bar.
export function ScopeBar({ children, managePath, actions }: { children: React.ReactNode; managePath: string; actions?: React.ReactNode }) {
  return (
    <div
      className="career-enter career-delay-1 mb-4 flex items-end gap-3 rounded-3xl border px-4 py-3 text-sm"
      style={{ borderColor: PAGE_ACCENT.soft, background: PAGE_ACCENT.soft }}
    >
      {children}
      <a
        href={managePath}
        onClick={(e) => {
          e.preventDefault()
          window.__cinqoToolBridge.navigate(managePath)
        }}
        className="pb-1 font-medium hover:underline"
        style={{ color: PAGE_ACCENT.text }}
      >
        Manage
      </a>
      {actions && <div className="ml-auto self-center">{actions}</div>}
    </div>
  )
}

// What a page shows in place of its content when the profile or persona it needs does
// not exist yet: a note with a link to where to create one.
export function ScopeNotice({ noun, path }: { noun: string; path: string }) {
  return (
    <div
      className="career-enter career-delay-1 mb-4 rounded-3xl border p-4 text-sm text-gray-700"
      style={{ borderColor: PAGE_ACCENT.soft, background: PAGE_ACCENT.soft }}
    >
      No {noun} yet.{' '}
      <a
        href={path}
        onClick={(e) => {
          e.preventDefault()
          window.__cinqoToolBridge.navigate(path)
        }}
        className="font-medium underline"
      >
        Create one to get started
      </a>
      .
    </div>
  )
}
