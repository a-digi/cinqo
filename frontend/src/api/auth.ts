import { apiGet, apiPost } from './client'

// The backend (api/src/auth/handler/*.go) returns snake_case JSON. These
// *Raw interfaces match the wire shape exactly; the exported functions
// below map them to this frontend's camelCase types.
//
// AuthConfig/AuthMeResponse are owned here (the API layer), not in a
// separate Components/Auth/types.ts — frontend/auth/01's AuthContext
// imports them from this file instead of the reverse, which keeps the
// dependency direction the usual way round (UI imports from api/, not
// api/ reaching into a UI-owned types file).

export interface AuthConfig {
  authorizeUrl: string
  clientId: string
  redirectUri: string
  scopes: string
  unrequestedScopes: string[]
}

export interface AuthMeResponse {
  userId: string
  scopes: string[]
  expiresAt: number
  email?: string
  name?: string
  preferredUsername?: string
}

interface AuthConfigRaw {
  authorize_url: string
  client_id: string
  redirect_uri: string
  scopes: string
  unrequested_scopes?: string[]
}

interface AuthMeRaw {
  user_id: string
  scopes: string[]
  expires_at: number
  email?: string
  name?: string
  preferred_username?: string
}

interface RenewResponse {
  status: string
  expires_in: number
}

interface StatusResponse {
  status: string
}

export async function fetchAuthConfig(): Promise<AuthConfig> {
  const raw = await apiGet<AuthConfigRaw>('/api/v1/auth/config')
  return {
    authorizeUrl: raw.authorize_url,
    clientId: raw.client_id,
    redirectUri: raw.redirect_uri,
    scopes: raw.scopes,
    unrequestedScopes: raw.unrequested_scopes ?? [],
  }
}

export async function fetchAuthMe(): Promise<AuthMeResponse> {
  const raw = await apiGet<AuthMeRaw>('/api/v1/auth/me')
  return {
    userId: raw.user_id,
    scopes: raw.scopes,
    expiresAt: raw.expires_at,
    email: raw.email,
    name: raw.name,
    preferredUsername: raw.preferred_username,
  }
}

export interface CompleteCallbackParams {
  code: string
  codeVerifier: string
  state?: string
}

// Completes the PKCE code exchange server-side. The browser arrives at
// AuthCallbackPage already carrying ?code= from coco-iam's redirect (via
// this same /auth/callback route, hit once already without a verifier —
// see the backend's callback.go two-branch design); this call supplies
// the verifier to complete the exchange and have the backend set the
// session cookies.
//
// Deliberately does NOT send redirect_uri: the backend defaults it to
// cfg.RedirectURI (the backend's own /auth/callback URL) when omitted,
// which is the value that was actually used in the original /authorize
// request — sending the frontend's own origin here instead would create
// an OAuth redirect_uri mismatch against coco-iam's token endpoint.
export async function completeAuthCallback(params: CompleteCallbackParams): Promise<void> {
  const query = new URLSearchParams({
    code: params.code,
    code_verifier: params.codeVerifier,
    ...(params.state ? { state: params.state } : {}),
  })
  await apiGet<StatusResponse>(`/auth/callback?${query.toString()}`)
}

export async function renewSession(): Promise<RenewResponse> {
  return apiPost<RenewResponse>('/api/v1/auth/renew')
}

export async function logout(): Promise<void> {
  await apiPost<StatusResponse>('/api/v1/auth/logout')
}
