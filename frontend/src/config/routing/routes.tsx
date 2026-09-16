import { createBrowserRouter } from 'react-router-dom'
import { AuthCallbackPage } from '../../Components/Auth/AuthCallbackPage'
import { AuthGuard } from '../../Components/Auth/AuthGuard'
import { AuthenticatedProviders } from '../../Components/Auth/AuthenticatedProviders'
import { LoginPage } from '../../Components/Auth/LoginPage'
import { Layout } from '../../Layout/Layout'
import { ScopesPage } from '../../Components/Admin/Security/ScopesPage'
import { ToolsListPage } from '../../Components/Admin/Tools/ToolsListPage'
import { PlatformKeysPage } from '../../Components/Admin/Platforms/PlatformKeysPage'
import { ConversationPage } from '../../Components/Conversation/ConversationPage'
import { AiLogsPage } from '../../Components/Conversation/AiLogsPage'
import { AiDebugSettingsPage } from '../../Components/Admin/Conversation/AiDebugSettingsPage'
import { AiLogsOverviewPage } from '../../Components/Admin/Conversation/AiLogsOverviewPage'
import { ToolRouteOutlet } from '../../Components/Tools/ToolRouteOutlet'
import { AppScopes } from '../security/scopes'

export const router = createBrowserRouter([
  {
    element: <AuthenticatedProviders />,
    children: [
      { path: '/login', element: <LoginPage /> },
      { path: '/login/callback', element: <AuthCallbackPage /> },
      {
        path: '/',
        element: (
          <AuthGuard>
            <Layout />
          </AuthGuard>
        ),
        children: [
          // Placeholder landing — replace with the first real page.
          { index: true, element: <div>cinqo — signed in</div> },
          {
            path: '/admin/security/scopes',
            element: (
              <AuthGuard scopes={[AppScopes.SuperAdmin]}>
                <ScopesPage />
              </AuthGuard>
            ),
          },
          {
            path: '/admin/tools',
            element: (
              <AuthGuard scopes={[AppScopes.ToolManage]}>
                <ToolsListPage />
              </AuthGuard>
            ),
          },
          {
            path: '/admin/platforms',
            element: (
              <AuthGuard scopes={[AppScopes.PlatformRead]}>
                <PlatformKeysPage />
              </AuthGuard>
            ),
          },
          {
            path: '/conversations',
            element: (
              <AuthGuard scopes={[AppScopes.ConversationUse]}>
                <ConversationPage />
              </AuthGuard>
            ),
          },
          {
            path: '/conversations/:id/logs',
            element: (
              <AuthGuard scopes={[AppScopes.ConversationUse]}>
                <AiLogsPage />
              </AuthGuard>
            ),
          },
          {
            path: '/admin/conversation-debug',
            element: (
              <AuthGuard scopes={[AppScopes.SuperAdmin]}>
                <AiDebugSettingsPage />
              </AuthGuard>
            ),
          },
          {
            path: '/admin/conversation-logs',
            element: (
              <AuthGuard scopes={[AppScopes.SuperAdmin]}>
                <AiLogsOverviewPage />
              </AuthGuard>
            ),
          },
          // Hosts every tool-registered route — a tool never gets a
          // real React Router route of its own. See
          // plan/ai/tools/step-07-frontend-bridge-and-menu-integration.md.
          { path: '/tools/*', element: <ToolRouteOutlet /> },
        ],
      },
    ],
  },
])
