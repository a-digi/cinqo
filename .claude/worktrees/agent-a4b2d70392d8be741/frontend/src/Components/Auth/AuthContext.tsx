import { createContext, useCallback, useContext, useEffect, useRef, useState, type ReactNode } from 'react'
import { fetchAuthConfig, fetchAuthMe, logout as logoutRequest, renewSession } from '../../api/auth'
import { ApiError } from '../../api/client'
import { onSessionExpired } from '../../api/sessionExpiry'
import { useSnackBar } from '../../Shared/Components/SnackBar/SnackBarContext'
import { generateCodeChallenge, generateCodeVerifier } from './pkce'
import type { AuthUser } from './types'

export const PKCE_VERIFIER_STORAGE_KEY = 'cinqo_pkce_verifier'

// How long before the access token expires to proactively renew it —
// matches coco-aim/plan/auth-integration.md's documented ~60s buffer.
const PROACTIVE_RENEW_BUFFER_SECONDS = 60

export interface AuthContextValue {
  isAuthenticated: boolean
  isLoading: boolean
  user: AuthUser | null
  scopes: string[]
  login: () => Promise<void>
  // `silent` suppresses the "Signed out." toast — used by an idle/wake
  // auto-logout (frontend/auth/03's optional SessionWatcher), which
  // shows its own reason instead.
  logout: (opts?: { silent?: boolean }) => Promise<void>
  refreshAuthState: () => Promise<void>
}

const AuthContext = createContext<AuthContextValue | null>(null)

export function AuthProvider({ children }: { children: ReactNode }) {
  const [isAuthenticated, setIsAuthenticated] = useState(false)
  const [isLoading, setIsLoading] = useState(true)
  const [user, setUser] = useState<AuthUser | null>(null)
  const [scopes, setScopes] = useState<string[]>([])
  const { errorMessage, successMessage } = useSnackBar()

  const renewTimer = useRef<ReturnType<typeof setTimeout> | null>(null)
  // Holds the latest refreshAuthState so the setTimeout callback (set up
  // once per schedule call) never closes over a stale version of it.
  const refreshAuthStateRef = useRef<() => Promise<void>>(() => Promise.resolve())
  // Latest isAuthenticated for the session-expiry subscriber (set up once), so it
  // never closes over a stale value and never fires on the pre-login bootstrap.
  const isAuthenticatedRef = useRef(false)

  const clearSession = useCallback(() => {
    setIsAuthenticated(false)
    setScopes([])
    setUser(null)
    if (renewTimer.current) {
      clearTimeout(renewTimer.current)
      renewTimer.current = null
    }
  }, [])

  const scheduleProactiveRenew = useCallback(
    (expiresAt: number) => {
      if (renewTimer.current) clearTimeout(renewTimer.current)
      const msUntilRenew = (expiresAt - PROACTIVE_RENEW_BUFFER_SECONDS) * 1000 - Date.now()
      if (msUntilRenew <= 0) return
      renewTimer.current = setTimeout(() => {
        void (async () => {
          try {
            await renewSession()
            await refreshAuthStateRef.current()
          } catch {
            clearSession()
            errorMessage('Your session has expired. Please sign in again.')
          }
        })()
      }, msUntilRenew)
    },
    [clearSession, errorMessage],
  )

  const refreshAuthState = useCallback(async () => {
    try {
      const me = await fetchAuthMe()
      setIsAuthenticated(true)
      setScopes(me.scopes)
      setUser({
        userId: me.userId,
        email: me.email,
        name: me.name,
        preferredUsername: me.preferredUsername,
      })
      scheduleProactiveRenew(me.expiresAt)
    } catch (err) {
      clearSession()
      // A 401 on the session check just means "not logged in" — expected
      // on a fresh visit, not an error worth surfacing to the user.
      if (!(err instanceof ApiError && err.status === 401)) {
        errorMessage('Your session has expired. Please sign in again.')
      }
    }
  }, [clearSession, errorMessage, scheduleProactiveRenew])

  useEffect(() => {
    refreshAuthStateRef.current = refreshAuthState
  }, [refreshAuthState])

  useEffect(() => {
    isAuthenticatedRef.current = isAuthenticated
  }, [isAuthenticated])

  // A dead-session 401 anywhere clears auth state so the mounted
  // AuthGuard redirects to /login — only when we were actually
  // authenticated, so the pre-login bootstrap 401 never triggers it.
  useEffect(() => {
    return onSessionExpired(() => {
      if (!isAuthenticatedRef.current) return
      clearSession()
      errorMessage('Your session has expired. Please sign in again.')
    })
  }, [clearSession, errorMessage])

  useEffect(() => {
    void refreshAuthState().finally(() => {
      setIsLoading(false)
    })
    return () => {
      if (renewTimer.current) clearTimeout(renewTimer.current)
    }
    // Runs once on mount only — refreshAuthState is re-created on every
    // render but refreshAuthStateRef (kept current by the effect above)
    // is what the proactive-renew timer actually calls.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const login = useCallback(async () => {
    try {
      const config = await fetchAuthConfig()
      const verifier = generateCodeVerifier()
      sessionStorage.setItem(PKCE_VERIFIER_STORAGE_KEY, verifier)
      const challenge = await generateCodeChallenge(verifier)
      const params = new URLSearchParams({
        response_type: 'code',
        client_id: config.clientId,
        redirect_uri: config.redirectUri,
        scope: config.scopes,
        code_challenge: challenge,
        code_challenge_method: 'S256',
      })
      window.location.href = `${config.authorizeUrl}?${params.toString()}`
    } catch (err) {
      errorMessage('Could not start sign-in. Please try again.')
      throw err
    }
  }, [errorMessage])

  const logout = useCallback(
    async (opts?: { silent?: boolean }) => {
      let failed = false
      try {
        await logoutRequest()
      } catch {
        failed = true
      } finally {
        // Always drop local auth — a failed server call must not leave the user
        // "logged in" client-side (the AuthGuard then redirects to /login).
        clearSession()
      }
      if (failed) {
        errorMessage('Sign-out failed, but you have been signed out locally.')
      } else if (!opts?.silent) {
        successMessage('Signed out.')
      }
    },
    [clearSession, errorMessage, successMessage],
  )

  const value: AuthContextValue = {
    isAuthenticated,
    isLoading,
    user,
    scopes,
    login,
    logout,
    refreshAuthState,
  }

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}

// Context + hook co-located deliberately (same convention as every other
// *Context.tsx in this codebase) — costs this file Fast Refresh for the
// hook specifically (editing useAuth alone forces a full reload instead of
// a hot patch), never a runtime issue.
// eslint-disable-next-line react-refresh/only-export-components
export function useAuth(): AuthContextValue {
  const ctx = useContext(AuthContext)
  if (!ctx) {
    throw new Error('useAuth must be used within an AuthProvider')
  }
  return ctx
}
