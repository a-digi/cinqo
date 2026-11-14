import { useLocation, useNavigate } from 'react-router-dom'
import { canGoBack } from '../config/breadcrumbs/breadcrumbRules'
import { IconButton } from '../Shared/Components/IconButton/IconButton'

// The first thing in the breadcrumb bar: one step back in the browser history. Disabled when this page is the first one the tab
// showed (opened directly or in a new tab): going back would leave Cinqo.
export function BackButton() {
  const navigate = useNavigate()
  // Read on every address change (useLocation re-renders us), so it turns on after the first in-app navigation.
  useLocation()
  return (
    <IconButton
      label="Go back"
      disabled={!canGoBack(window.history.state)}
      onClick={() => {
        void navigate(-1)
      }}
      icon={
        <svg viewBox="0 0 20 20" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" className="h-4 w-4" aria-hidden="true">
          <path d="M12 4 6 10l6 6" />
        </svg>
      }
    />
  )
}
