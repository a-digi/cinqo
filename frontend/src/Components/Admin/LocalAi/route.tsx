import type { RouteObject } from 'react-router-dom'
import { AuthGuard } from '../../Auth/AuthGuard'
import { AppScopes } from '../../../config/security/scopes'
import { LocalAiPage } from './LocalAiPage'

// The Local AI page's route, registered by one line in config/routing/routes.tsx.
export const localAiRoute: RouteObject = {
  path: '/admin/local-ai',
  element: (
    <AuthGuard scopes={[AppScopes.PlatformRead]}>
      <LocalAiPage />
    </AuthGuard>
  ),
}
