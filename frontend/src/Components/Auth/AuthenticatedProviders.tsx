import { Outlet } from 'react-router-dom'
import { AuthProvider } from './AuthContext'
import { ToolRegistryProvider } from '../../config/tools/ToolRegistryContext'
import { ConversationProvider } from '../../config/conversation/ConversationContext'

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
// as coco-mda accumulates them. ToolRegistryProvider (plan/ai/tools/
// step-07) is the first one — it needs useAuth() itself (only loads
// tools once truly authenticated), so it must nest inside AuthProvider,
// not beside it. ConversationProvider (plan/ai/conversation/step-16) is
// the second — same reasoning, and it must be visible to both
// ConversationPage (rendered under Layout's own Outlet) and, from
// step-17, the floating widget mounted directly in Layout — the only
// placement visible to both.
export function AuthenticatedProviders() {
  return (
    <AuthProvider>
      <ToolRegistryProvider>
        <ConversationProvider>
          <Outlet />
        </ConversationProvider>
      </ToolRegistryProvider>
    </AuthProvider>
  )
}
