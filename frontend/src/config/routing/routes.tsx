import { createBrowserRouter } from 'react-router-dom'
import { AuthCallbackPage } from '../../Components/Auth/AuthCallbackPage'
import { AuthGuard } from '../../Components/Auth/AuthGuard'
import { AuthenticatedProviders } from '../../Components/Auth/AuthenticatedProviders'
import { LoginPage } from '../../Components/Auth/LoginPage'
import { Layout } from '../../Layout/Layout'

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
        ],
      },
    ],
  },
])
