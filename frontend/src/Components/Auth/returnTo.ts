// Where to go after signing in: the page the user asked for before the guard sent them to /login (for example a pasted
// /conversations/<uuid> link). Kept in sessionStorage because the sign-in leaves the app and comes back in the same tab, like the PKCE verifier.
export const RETURN_TO_STORAGE_KEY = 'cinqo_return_to'

const DEFAULT_RETURN_TO = '/'

// Only a path inside this app is accepted: it must start with exactly one slash, so `//host/x`, `/\host`, `https://…` and `javascript:…`
// can never turn the post-login redirect into a jump to another site. The sign-in pages themselves are excluded (no loop).
export function safeReturnTo(value: unknown): string {
  if (typeof value !== 'string') return DEFAULT_RETURN_TO
  if (!/^\/(?![/\\])[^\s\\]*$/.test(value)) return DEFAULT_RETURN_TO
  const path = value.split(/[?#]/)[0]
  if (path === '/login' || path.startsWith('/login/')) return DEFAULT_RETURN_TO
  return value
}

export function rememberReturnTo(value: unknown): void {
  try {
    const target = safeReturnTo(value)
    if (target === DEFAULT_RETURN_TO) sessionStorage.removeItem(RETURN_TO_STORAGE_KEY)
    else sessionStorage.setItem(RETURN_TO_STORAGE_KEY, target)
  } catch {
    // storage unavailable: the user lands on the home page after sign-in, as before
  }
}

// Reads and forgets the remembered target (one use only).
export function takeReturnTo(): string {
  try {
    const value = sessionStorage.getItem(RETURN_TO_STORAGE_KEY)
    sessionStorage.removeItem(RETURN_TO_STORAGE_KEY)
    return safeReturnTo(value)
  } catch {
    return DEFAULT_RETURN_TO
  }
}
