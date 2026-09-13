import { useErrorAlert } from '../Shared/Components/ErrorAlert/ErrorAlertContext'
import { ErrorAlert } from '../Shared/Components/ErrorAlert/ErrorAlert'
import { Content } from './Content'
import { Navbar } from './Navbar'
import { Sidebar } from './Sidebar'

// Minimal authenticated shell: Navbar + Sidebar + Content, nothing else.
// coco-mda's own Layout accumulated a TopBar, Footer, collapsible
// sidebar, AI chat widget, and session-security provider over its whole
// lifetime — none of that exists yet for cinqo's skeleton; add pieces
// here only once a real feature needs them.
export function Layout() {
  const { errors, dismissError } = useErrorAlert()

  return (
    <div className="flex h-screen flex-col overflow-hidden bg-gray-50">
      <Navbar />
      <ErrorAlert errors={errors} onDismiss={dismissError} />
      <div className="flex flex-1 overflow-hidden">
        <Sidebar />
        <Content />
      </div>
    </div>
  )
}
