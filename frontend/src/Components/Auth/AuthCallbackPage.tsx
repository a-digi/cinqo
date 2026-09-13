import { useEffect, useRef, useState } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { completeAuthCallback } from '../../api/auth'
import { LoadingSpinner } from '../../Shared/Components/Loading/LoadingSpinner'
import { useSnackBar } from '../../Shared/Components/SnackBar/SnackBarContext'
import { PKCE_VERIFIER_STORAGE_KEY, useAuth } from './AuthContext'

export function AuthCallbackPage() {
  const [status, setStatus] = useState<'pending' | 'error'>('pending')
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const { refreshAuthState } = useAuth()
  const { errorMessage } = useSnackBar()
  // The authorization code is single-use — coco-iam rejects a second
  // token-exchange attempt for the same code. React.StrictMode
  // double-invokes this effect in dev (mount, cleanup, mount), which
  // would otherwise fire completeAuthCallback twice for the same code
  // and always lose the race on one of them. This ref survives the
  // StrictMode remount (same component instance) so only the first
  // invocation ever runs the exchange.
  //
  // Deliberately no "cancelled" flag guarding setStatus/navigate below:
  // StrictMode's cleanup for the first (real) invocation fires
  // synchronously right after mount, before completeAuthCallback's
  // promise resolves — a cancelled flag tied to that invocation would go
  // true before the exchange even finishes, permanently blocking the
  // navigate() on completion even though the login genuinely succeeded.
  // startedRef is the only guard needed since it correctly identifies
  // the one invocation that's actually allowed to run.
  const startedRef = useRef(false)

  useEffect(() => {
    async function completeLogin() {
      if (startedRef.current) return
      startedRef.current = true

      const code = searchParams.get('code')
      const state = searchParams.get('state') ?? undefined
      const verifier = sessionStorage.getItem(PKCE_VERIFIER_STORAGE_KEY)

      if (!code || !verifier) {
        setStatus('error')
        errorMessage('Sign-in failed. Please try again.')
        return
      }

      try {
        await completeAuthCallback({ code, codeVerifier: verifier, state })
        sessionStorage.removeItem(PKCE_VERIFIER_STORAGE_KEY)
        await refreshAuthState()
        navigate('/', { replace: true })
      } catch {
        setStatus('error')
        errorMessage('Sign-in failed. Please try again.')
      }
    }

    void completeLogin()
    // Runs once on mount — reading the code/verifier from the URL and
    // sessionStorage at the moment this page loads is the entire point.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  if (status === 'error') {
    return (
      <div className="flex min-h-screen flex-col items-center justify-center gap-4 bg-slate-50">
        <p className="text-sm text-slate-700">Something went wrong completing sign-in.</p>
        <a href="/login" className="text-sm font-medium text-slate-900 underline">
          Back to sign in
        </a>
      </div>
    )
  }

  return (
    <div className="flex min-h-screen items-center justify-center bg-slate-50">
      <LoadingSpinner label="Signing in…" />
    </div>
  )
}
