import type { ReactNode } from 'react'
import { useAuth } from '../../../Components/Auth/AuthContext'
import { AppScopes } from '../../../config/security/scopes'

// For gating a UI element narrower than a whole route (a button, a menu
// entry) — AuthGuard already covers whole-route gating. This is a UX
// convenience only: hiding an element behind ScopeGate does not stop a
// determined caller from hitting the underlying API directly — the
// backend's own scope check is the actual security boundary.
export function ScopeGate({ scopes, children }: { scopes: string[]; children: ReactNode }) {
  const { scopes: userScopes } = useAuth()
  const hasAccess = userScopes.includes(AppScopes.SuperAdmin) || scopes.some((s) => userScopes.includes(s))
  return hasAccess ? <>{children}</> : null
}
