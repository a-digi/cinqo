import { Outlet } from 'react-router-dom'
import { AuthProvider } from './AuthContext'

// Pathless layout route wrapping every route that needs a session
// (/login, /login/callback, and the AuthGuard-protected `/` subtree).
//
// Critical placement rule, called out because it was a real regression
// in coco-mda: if cinqo ever adds a genuinely public, token-authenticated
// route (e.g. a public share link), it must be a SIBLING of this element
// in the router (frontend/step-04), never a child — otherwise
// AuthProvider's mount-time GET /api/v1/auth/me fires for a fully
// anonymous visitor with no session to check.
//
// Any future auth-dependent global provider (a notification context, a
// profile-avatar context, etc.) nests inside AuthProvider here, exactly
// as coco-mda accumulates them — none exist yet for cinqo's skeleton.
export function AuthenticatedProviders() {
  return (
    <AuthProvider>
      <Outlet />
    </AuthProvider>
  )
}
