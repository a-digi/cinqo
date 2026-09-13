import { RouterProvider } from 'react-router-dom'
import { HttpClientProvider } from './api/http/HttpClient'
import { SnackBarProvider } from './Shared/Components/SnackBar/SnackBarContext'
import { ErrorAlertProvider } from './Shared/Components/ErrorAlert/ErrorAlertContext'
import { router } from './config/routing/routes'

// Only providers that are route-agnostic and auth-independent live here —
// anything auth-scoped lives in AuthenticatedProviders instead, so a
// fully public/anonymous route (cinqo has none yet, but the pattern must
// hold if one is ever added) never mounts AuthProvider and never fires
// GET /api/v1/auth/me needlessly.
function App() {
  return (
    <SnackBarProvider>
      <ErrorAlertProvider>
        <HttpClientProvider>
          <RouterProvider router={router} />
        </HttpClientProvider>
      </ErrorAlertProvider>
    </SnackBarProvider>
  )
}

export default App
