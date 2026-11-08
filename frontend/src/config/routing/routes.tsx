import { createBrowserRouter, Navigate } from 'react-router-dom'
import { AuthCallbackPage } from '../../Components/Auth/AuthCallbackPage'
import { AuthGuard } from '../../Components/Auth/AuthGuard'
import { AuthenticatedProviders } from '../../Components/Auth/AuthenticatedProviders'
import { LoginPage } from '../../Components/Auth/LoginPage'
import { Layout } from '../../Layout/Layout'
import { ScopesPage } from '../../Components/Admin/Security/ScopesPage'
import { ToolsListPage } from '../../Components/Admin/Tools/ToolsListPage'
import { RepositoriesPage } from '../../Components/Admin/Tools/RepositoriesPage'
import { RepositoryPage } from '../../Components/Admin/Tools/RepositoryPage'
import { PlatformKeysPage } from '../../Components/Admin/Platforms/PlatformKeysPage'
import { PlatformsPage } from '../../Components/Admin/Platforms/PlatformsPage'
import { localAiRoute } from '../../Components/Admin/LocalAi/route'
import { ConversationPage } from '../../Components/Conversation/ConversationPage'
import { AiLogsPage } from '../../Components/Conversation/AiLogsPage'
import { AiLogsOverviewPage } from '../../Components/Admin/Conversation/AiLogsOverviewPage'
import { HomePage } from '../../Components/Home/HomePage'
import { MediaListPage } from '../../Components/Admin/Media/MediaListPage'
import { ToolRouteOutlet } from '../../Components/Tools/ToolRouteOutlet'
import { ToolLogReaderPage } from '../../Components/FileReader/ToolLogReaderPage'
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
          { index: true, element: <HomePage /> },
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
            path: '/admin/tool-repositories',
            element: (
              <AuthGuard scopes={[AppScopes.ToolManage]}>
                <RepositoriesPage />
              </AuthGuard>
            ),
          },
          {
            path: '/admin/tool-repositories/:id',
            element: (
              <AuthGuard scopes={[AppScopes.ToolManage]}>
                <RepositoryPage />
              </AuthGuard>
            ),
          },
          {
            path: '/admin/platforms',
            element: (
              <AuthGuard scopes={[AppScopes.PlatformRead]}>
                <PlatformsPage />
              </AuthGuard>
            ),
          },
          {
            path: '/admin/api-keys',
            element: (
              <AuthGuard scopes={[AppScopes.PlatformRead]}>
                <PlatformKeysPage />
              </AuthGuard>
            ),
          },
          localAiRoute,
          {
            // One route for the list and for one conversation (`/conversations/<uuid>`), so the page keeps its own state between the two
            // (a second route with the same element would remount it). `/conversations/:id/logs` below has more segments and is separate.
            path: '/conversations/:id?',
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
            // The AI logging switch moved into the Logs page (plan/ai/conversation/step-44); the old address still works.
            path: '/admin/conversation-debug',
            element: <Navigate to="/admin/conversation-logs" replace />,
          },
          {
            path: '/admin/conversation-logs',
            element: (
              <AuthGuard scopes={[AppScopes.SuperAdmin]}>
                <AiLogsOverviewPage />
              </AuthGuard>
            ),
          },
          {
            // plan 201: the log reader for installed tools. Signed-in people only: which files a person may read is decided by the server per
            // source and scope on every request, so a static scope here would be wrong (like the file reader's own API).
            path: '/logs/read',
            element: <ToolLogReaderPage />,
          },
          {
            path: '/admin/media',
            element: (
              <AuthGuard scopes={[AppScopes.SuperAdmin]}>
                <MediaListPage />
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
