// loginSession.ts (plan 195, step 2) — keeping a login session for a link. When the person logs in in the window, they may tick "Keep this
// login": the Browser tool then saves the SITE'S SESSION (cookies, sealed, never the password) and this page only gets an opaque id, which
// Career stores on the link. This file never sees a cookie or a password: it sends the opt-in with the confirmation and passes ids around.

const BROWSER_CONFIRM_URL = '/api/v1/tools/browser/proxy/login-window/confirm'
const BROWSER_SESSION_STATE_URL = '/api/v1/tools/browser/proxy/login-session/state'
const CAREER_LINK_SESSION_URL = '/api/v1/tools/career/proxy/portal-links/login-session'

export const KEEP_LOGIN_LABEL = 'Keep this login for Katana'

export const KEEP_LOGIN_EXPLANATION =
  'Saves the site’s login session (not your password) encrypted on this computer, for at most 30 days, so Katana can crawl this link without a login window while it lasts. ' +
  'Many sites forbid automated access to a logged-in account and may restrict it: use this at your own risk. You can forget it at any time.'

export const KEEP_LOGIN_NOT_USED_NOTE = 'The selected engine does not use a kept login: it is only used while Katana is selected (Browser & Engine).'

// keepLoginExplanation: the text under the checkbox; engineUsesIt says whether the selected engine reads a link with a kept login (Katana).
export function keepLoginExplanation(engineUsesIt: boolean): string {
  return engineUsesIt ? KEEP_LOGIN_EXPLANATION : `${KEEP_LOGIN_EXPLANATION} ${KEEP_LOGIN_NOT_USED_NOTE}`
}

export const NOT_KEPT_NOTE = 'You type your login on the site itself; nothing of it is kept here unless you tick “Keep this login”.'

export interface LoginSessionInfo {
  id: string
  host: string
  state: 'valid' | 'expired'
  capturedAt: string
  expiresAt: string
  lastOkAt?: string
}

export interface ConfirmResult {
  session: LoginSessionInfo | null
  sessionError: string
}

// confirmErrorText turns the Browser's refusal of a confirmation into one sentence for the person. The Browser's own text names the reason
// (the page still shows a login form, it is a bot check, the page is empty); a 404 means the window is gone.
export function confirmErrorText(status: number, body: string): string {
  if (status === 404) return 'The login window is closed. Start the crawl again to get a new one.'
  const text = body.trim()
  if (text) return text
  return `Could not confirm the login (${status.toString()}).`
}

// confirmLoginKeeping asks the Browser tool to check the page in the run's window and mark the login as confirmed; with keepSession it also
// keeps the site's session. A session that could not be kept never fails the confirmation: the login is fine, sessionError says why not.
export async function confirmLoginKeeping(runId: string, keepSession: boolean): Promise<ConfirmResult> {
  const res = await fetch(BROWSER_CONFIRM_URL, {
    method: 'POST',
    credentials: 'include',
    body: JSON.stringify(keepSession ? { requestId: runId, keepSession: true } : { requestId: runId }),
  })
  if (!res.ok) {
    const text = await res.text().catch(() => '')
    throw new Error(confirmErrorText(res.status, text))
  }
  const body = (await res.json().catch(() => ({}))) as { session?: LoginSessionInfo; sessionError?: string }
  return { session: body.session ?? null, sessionError: body.sessionError ?? '' }
}

// attachLoginSession stores the opaque id on the link (the older session of the link is forgotten by the server).
export async function attachLoginSession(linkId: string, sessionId: string): Promise<void> {
  const res = await fetch(CAREER_LINK_SESSION_URL, {
    method: 'PUT',
    credentials: 'include',
    body: JSON.stringify({ linkId, sessionId }),
  })
  if (!res.ok) {
    const text = await res.text().catch(() => '')
    throw new Error(text.trim() || `Could not save the login for this link (${res.status.toString()}).`)
  }
}

// fetchLinkLoginSession returns the link's kept login (with its state from the Browser tool), or null when the link has none.
export async function fetchLinkLoginSession(linkId: string): Promise<LoginSessionInfo | null> {
  const res = await fetch(`${CAREER_LINK_SESSION_URL}?linkId=${encodeURIComponent(linkId)}`, { credentials: 'include' })
  if (!res.ok) return null
  const { sessionId } = (await res.json()) as { sessionId?: string }
  if (!sessionId) return null
  const state = await fetch(`${BROWSER_SESSION_STATE_URL}?id=${encodeURIComponent(sessionId)}`, { credentials: 'include' })
  if (state.status === 404) return null // forgotten or deleted in the Browser tool
  if (!state.ok) return null
  return (await state.json()) as LoginSessionInfo
}

// forgetLinkLoginSession clears the link's kept login; the server has the Browser tool forget the session.
export async function forgetLinkLoginSession(linkId: string): Promise<void> {
  const res = await fetch(`${CAREER_LINK_SESSION_URL}?linkId=${encodeURIComponent(linkId)}`, { method: 'DELETE', credentials: 'include' })
  if (!res.ok && res.status !== 404) throw new Error(`Could not forget the login (${res.status.toString()}).`)
}

function dayText(iso: string): string {
  const d = new Date(iso)
  return Number.isNaN(d.getTime()) ? iso : d.toLocaleDateString(undefined, { year: 'numeric', month: 'short', day: 'numeric' })
}

// keptLoginText: the one line that says what is kept for a link.
export function keptLoginText(info: LoginSessionInfo): string {
  if (info.state === 'expired') return `The kept login for ${info.host} has expired. Log in again and keep the login (Crawl now or Generate opens the window).`
  return `Login kept for ${info.host} until ${dayText(info.expiresAt)}.`
}

export const FORGET_LOGIN_LABEL = 'Forget this login'

// keepResultNote: what to tell the person after a confirmation that asked to keep the login, or null when nothing needs saying.
export function keepResultNote(asked: boolean, result: ConfirmResult): string | null {
  if (!asked) return null
  if (result.session) return `Login kept until ${dayText(result.session.expiresAt)}.`
  return result.sessionError ? `The login works, but it could not be kept: ${result.sessionError}.` : null
}
