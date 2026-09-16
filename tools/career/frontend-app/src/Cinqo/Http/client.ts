// client.ts — the generic HTTP/JSON client plumbing every cinqo CORE
// API slice shares (coreApi.ts's own Platform slice,
// Cinqo/Conversation/conversation.ts's own Conversation slice):
// httpClient (get/post/patch/delete) prepares the request-side options
// every call site was repeating (credentials, JSON headers/body), and
// a typed error carrying the HTTP status, a plain success check
// (isSuccess), and responseBody take care of the response side —
// parsing a fetch Response's own JSON envelope and either returning
// its `message` payload or throwing a CoreApiError built from
// isSuccess's own verdict. Split out of coreApi.ts, step 45; httpClient
// added step 46. No slice-specific knowledge lives here — see
// coreApi.ts's own top comment for the shared auth/scope model every
// caller of this client relies on.

// Carries the HTTP status so callers can special-case 401/403 (e.g. a
// caller with tool:career:portals but neither cinqo:platform:read nor
// cinqo:conversation:use — step 24's own open question 4, resolved in
// step 25 with a friendlier message keyed off this field) without
// parsing it back out of a generic error message.
export class CoreApiError extends Error {
  status: number
  constructor(status: number, message: string) {
    super(message)
    this.status = status
  }
}

// HttpClientOptions.headers lets a caller add a header beyond the
// credentials/JSON ones httpClient already sets on every request
// (e.g. a future If-Match) — not exercised by any call site yet, but
// a REST client with no way to ever add a header would be an odd
// dead end.
export interface HttpClientOptions {
  headers?: Record<string, string>
}

function request(method: string, url: string, body: unknown | undefined, options?: HttpClientOptions): Promise<Response> {
  const headers: Record<string, string> = { ...options?.headers }
  const init: RequestInit = { method, credentials: 'include', headers }
  if (body !== undefined) {
    headers['Content-Type'] = 'application/json'
    init.body = JSON.stringify(body)
  }
  return fetch(url, init)
}

// httpClient — the one place every cinqo CORE API call prepares its
// own request options: credentials: 'include' always (this tool's
// bundle runs same-origin, so the session cookie is what actually
// authenticates every one of these calls), and a JSON
// Content-Type/body only when a body is actually passed (get/delete
// never take one). isSuccess/responseBody handle the response side —
// responseBody parses a Response's own JSON envelope and either
// returns its `message` payload or throws a CoreApiError built from
// isSuccess's own verdict.
export const httpClient = {
  get(url: string, options?: HttpClientOptions): Promise<Response> {
    return request('GET', url, undefined, options)
  },
  post(url: string, body?: unknown, options?: HttpClientOptions): Promise<Response> {
    return request('POST', url, body, options)
  },
  patch(url: string, body?: unknown, options?: HttpClientOptions): Promise<Response> {
    return request('PATCH', url, body, options)
  },
  delete(url: string, options?: HttpClientOptions): Promise<Response> {
    return request('DELETE', url, undefined, options)
  },
  isSuccess(res: Response): boolean {
    return res.ok
  },
  async responseBody<T>(res: Response, action: string): Promise<T> {
    const body = (await res.json().catch(() => undefined)) as { message?: unknown } | undefined

    if (!this.isSuccess(res)) {
      const message = body && typeof body.message === 'string' ? body.message : `failed to ${action} (${res.status})`
      throw new CoreApiError(res.status, message)
    }

    return (body as { message: T }).message
  },
}
