import { Navigate } from 'react-router-dom'
import type { ReactNode } from 'react'
import { useAuth } from './AuthContext'
import { LoadingSpinner } from '../../Shared/Components/Loading/LoadingSpinner'
import { AccessDeniedPage } from '../../Shared/Components/Access/AccessDeniedPage'
import { AppScopes } from '../../config/security/scopes'

export interface AuthGuardProps {
  children: ReactNode
  scopes?: string[]
  // Rendered in place when `scopes` doesn't match. Defaults to
  // AccessDeniedPage — no /unauthorized route exists here, so rendering
  // inline avoids inventing one just for this.
  fallback?: ReactNode
}

export function AuthGuard({ children, scopes, fallback }: AuthGuardProps) {
  const { isAuthenticated, isLoading, scopes: userScopes } = useAuth()

  if (isLoading) {
    return (
      <div className="flex min-h-screen items-center justify-center">
        <LoadingSpinner label="Loading…" />
      </div>
    )
  }

  if (!isAuthenticated) {
    return <Navigate to="/login" replace />
  }

  if (scopes && scopes.length > 0) {
    const isSuperAdmin = userScopes.includes(AppScopes.SuperAdmin)
    const hasAccess = isSuperAdmin || scopes.some((scope) => userScopes.includes(scope))
    if (!hasAccess) {
      return <>{fallback ?? <AccessDeniedPage />}</>
    }
  }

  return <>{children}</>
}
