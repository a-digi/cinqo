// Fetch wrapper — credentials: 'include' on every call, since auth is
// httpOnly-cookie-based, never a client-side bearer token. On a 401,
// attempts one session renewal then retries the original request once
// before giving up. api/http/HttpClient.tsx wraps these primitives for
// real CRUD consumers — this file itself stays the low-level fetch
// layer, also used directly by api/auth.ts.
import { emitSessionExpired } from './sessionExpiry'

const RENEW_ENDPOINT = '/api/v1/auth/renew'

// A 401 on an auth endpoint (/auth/me on a fresh visit, /auth/renew, /auth/config)
// just means "not logged in yet" — it must NOT be treated as an expired session,
// or every first visit would fire the session-expiry logout. Only non-auth
// (data) requests whose 401 survives the renew signal a truly dead session.
function isAuthEndpoint(path: string): boolean {
  return path.startsWith('/api/v1/auth/') || path.startsWith('/auth/')
}

export class ApiError extends Error {
  status: number
  // Parsed response body, when the failed response was JSON — e.g. the
  // backend's {error:true, message:"..."} envelope, so a caller can
  // surface the backend's actual message instead of a generic one.
  body: unknown

  constructor(status: number, message: string, body?: unknown) {
    super(message)
    this.status = status
    this.body = body
  }
}

function errorMessageFromBody(body: unknown, fallback: string): string {
  if (body && typeof body === 'object' && 'message' in body && typeof body.message === 'string') {
    return (body as { message: string }).message
  }
  return fallback
}

// Reads the body as text first and only parses it if non-empty — an
// empty body is valid for any status code (not just 204; a handler that
// forgets to write one on 200/201 is a real bug class), and resp.json()
// throws a SyntaxError on "" that a caller would otherwise have to catch
// individually every time.
async function readJsonBody<T>(resp: Response): Promise<T> {
  const text = await resp.text()
  if (text === '') return undefined as T
  return JSON.parse(text) as T
}

async function rawRequest(path: string, init: RequestInit): Promise<Response> {
  // A FormData body (multipart upload) must NOT get an explicit
  // Content-Type — the browser sets its own, including the multipart
  // boundary, only when Content-Type is left unset.
  const isFormData = init.body instanceof FormData
  // init.headers is typed HeadersInit — a plain object, a Headers
  // instance, or a string[][] — so it's built via the Headers
  // constructor (which handles all three) rather than object-spread,
  // which would silently produce garbage for the latter two shapes.
  const headers = new Headers(init.headers)
  if (!isFormData && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json')
  }
  return fetch(path, {
    ...init,
    credentials: 'include',
    headers,
  })
}

async function renewSession(): Promise<boolean> {
  const resp = await rawRequest(RENEW_ENDPOINT, { method: 'POST' })
  return resp.ok
}

async function request<T>(path: string, init: RequestInit = {}, isRetry = false): Promise<T> {
  const resp = await rawRequest(path, init)

  if (resp.status === 401 && !isRetry) {
    const renewed = await renewSession()
    if (renewed) {
      return request<T>(path, init, true)
    }
    // Renew failed → the session is dead. Signal it (unless this IS an auth
    // endpoint, where a 401 just means "not logged in") so AuthContext can clear
    // state and the AuthGuards redirect to /login.
    if (!isAuthEndpoint(path)) {
      emitSessionExpired()
    }
  } else if (resp.status === 401 && isRetry && !isAuthEndpoint(path)) {
    // The retry after a "successful" renew still 401'd → session is dead.
    emitSessionExpired()
  }

  if (!resp.ok) {
    let body: unknown
    try {
      body = await readJsonBody(resp)
    } catch {
      body = undefined
    }
    const fallback = `Request to ${path} failed with status ${resp.status}`
    throw new ApiError(resp.status, errorMessageFromBody(body, fallback), body)
  }

  return readJsonBody<T>(resp)
}

export function apiGet<T>(path: string): Promise<T> {
  return request<T>(path, { method: 'GET' })
}

export function apiPost<T>(path: string, body?: unknown): Promise<T> {
  return request<T>(path, {
    method: 'POST',
    body: body !== undefined ? JSON.stringify(body) : undefined,
  })
}

export function apiPut<T>(path: string, body?: unknown): Promise<T> {
  return request<T>(path, {
    method: 'PUT',
    body: body !== undefined ? JSON.stringify(body) : undefined,
  })
}

export function apiPatch<T>(path: string, body?: unknown): Promise<T> {
  return request<T>(path, {
    method: 'PATCH',
    body: body !== undefined ? JSON.stringify(body) : undefined,
  })
}

export function apiDelete<T>(path: string): Promise<T> {
  return request<T>(path, { method: 'DELETE' })
}

// Multipart file upload — body is a FormData, never JSON.stringify'd.
export function apiUpload<T>(path: string, formData: FormData): Promise<T> {
  return request<T>(path, { method: 'POST', body: formData })
}
