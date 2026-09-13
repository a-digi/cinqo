import { useState } from 'react'
import { useErrorAlert } from '../Shared/Components/ErrorAlert/ErrorAlertContext'
import { ErrorAlert } from '../Shared/Components/ErrorAlert/ErrorAlert'
import { Content } from './Content'
import { Navbar } from './Navbar'
import { Sidebar } from './Sidebar'

// Minimal authenticated shell: Navbar + Sidebar + Content, nothing else
// beyond the sidebar's own show/hide toggle
// (plan/ai/frontend/frontend/step-08-collapsible-sidebars.md). State
// lives here, not in Sidebar itself — the control that re-expands a
// hidden sidebar (Navbar's toggle button) must stay rendered
// regardless of collapse state.
export function Layout() {
  const { errors, dismissError } = useErrorAlert()
  const [isSidebarCollapsed, setIsSidebarCollapsed] = useState(false)

  return (
    <div className="flex h-screen flex-col overflow-hidden bg-gray-50">
      <Navbar onToggleSidebar={() => setIsSidebarCollapsed((v) => !v)} />
      <ErrorAlert errors={errors} onDismiss={dismissError} />
      <div className="flex flex-1 overflow-hidden">
        {!isSidebarCollapsed && <Sidebar />}
        <Content />
      </div>
    </div>
  )
}
