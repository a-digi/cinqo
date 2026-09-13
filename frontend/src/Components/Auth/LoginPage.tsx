import { useEffect, useRef, useState } from 'react'
import { useAuth } from './AuthContext'
import { LoadingSpinner } from '../../Shared/Components/Loading/LoadingSpinner'

// No form fields, no button — cinqo has no password to collect.
// login() (auth/01) does the entire PKCE + redirect dance; this
// component triggers it automatically on mount rather than waiting for
// a click. See plan/ai/frontend/auth/step-05-auto-redirect-to-sign-in.md.
export function LoginPage() {
  const { login } = useAuth()
  const [error, setError] = useState<string | null>(null)
  // Guards against firing login() twice from React 18 StrictMode's
  // dev-only double-invoke of effects — a ref, not state, so it
  // doesn't itself trigger a re-render or need a cleanup function
  // racing the redirect.
  const attempted = useRef(false)

  const attemptLogin = () => {
    attempted.current = true
    setError(null)
    void login().catch(() => setError('Could not start sign-in.'))
  }

  useEffect(() => {
    if (attempted.current) return
    attemptLogin()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  if (error) {
    return (
      <div className="flex min-h-screen flex-col items-center justify-center gap-3">
        <p className="text-sm text-red-600">{error}</p>
        <button
          type="button"
          onClick={attemptLogin}
          className="rounded-md bg-slate-900 px-4 py-2 text-sm font-medium text-white hover:bg-slate-800"
        >
          Retry
        </button>
      </div>
    )
  }

  return (
    <div className="flex min-h-screen items-center justify-center">
      <LoadingSpinner label="Redirecting to sign in…" />
    </div>
  )
}
