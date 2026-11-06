// loginWindow.ts (plan 182, step 5) — what the Career page needs for a crawl that waits for a login: when a run is waiting, what to tell the
// person, and the one call that confirms the login. The person logs in on the site's own page in the window the Browser tool opened; this file
// never sees or sends a password, a cookie or anything typed there, and keeps nothing: the confirmation is only the run's id. The run itself is
// stopped with the existing stopCrawlNow (the run closes its window).

import type { CrawlRun } from './crawlNow'
import { confirmLoginKeeping } from './loginSession'

export const WAITING_FOR_LOGIN = 'waiting_for_login'

// isWaitingForLogin: a run that is still going and waits for the person to log in and confirm.
export function isWaitingForLogin(run: Pick<CrawlRun, 'status' | 'phase'> | null): boolean {
  return run !== null && run.status === 'running' && run.phase === WAITING_FOR_LOGIN
}

export const LOGIN_BADGE = 'Login needed'
export const LOGIN_BADGE_TOOLTIP =
  'This page needs you to log in. A crawl opens a window for you to log in on the site itself (unless a login is kept for Katana), so it is never crawled automatically.'

// loginBadgeFor: the badge text of a link, or null when it needs no login.
export function loginBadgeFor(link: { requiresLogin?: boolean }): string | null {
  return link.requiresLogin === true ? LOGIN_BADGE : null
}

// A link that needs a login is crawled BY HAND only: the person has to log in in the window a crawl opens, so nothing may start such a crawl
// on its own (the server enforces it too: auto discovery, the request flags and their repair all skip these links).
export const MANUAL_ONLY_NOTE = 'Crawled by hand only: this link needs a login, so it is never crawled automatically.'

// manualOnlyNote: the note to show in place of the Auto-discovery switch of a link that needs a login, or null for any other link.
export function manualOnlyNote(link: { requiresLogin?: boolean }): string | null {
  return link.requiresLogin === true ? MANUAL_ONLY_NOTE : null
}

export const LOGIN_BLOCKED_BY_AUTO_NOTE = 'Turn Auto-discovery off for this link first: a link that needs a login is crawled by hand only.'

// loginCheckboxState: the "Requires login" box of a link that has auto-crawl (Auto-discovery) on is disabled, with the reason, until
// Auto-discovery is turned off. A box that is already ticked can always be unticked. The server refuses both on at once as well.
export function loginCheckboxState(link: { autoDiscoveryEnabled?: boolean }, checked: boolean): { disabled: boolean; note: string | null } {
  const blocked = link.autoDiscoveryEnabled === true && !checked
  return { disabled: blocked, note: blocked ? LOGIN_BLOCKED_BY_AUTO_NOTE : null }
}

// plan 183: Generate with AI on a link that needs a login. It opens a window for the person to log in in (the window belongs to the AI conversation
// the Generate creates), waits for "I am logged in", and only then starts the AI, whose browser tools read the pages in that window. Nothing the
// person types is seen or kept by this page; the window is closed when the run ends or the person cancels.
export const GENERATE_LOGIN_NOTE =
  'This link needs a login. Generate opens a window for you to log in on the site itself; the AI then reads the pages in that window. Nothing of your login is kept unless you choose to keep it.'

// generateLoginNote: the note to show with Generate for a link that needs a login, or null for any other link.
export function generateLoginNote(link: { requiresLogin?: boolean }): string | null {
  return link.requiresLogin === true ? GENERATE_LOGIN_NOTE : null
}

export const AI_LOGIN_WAIT_TEXT = 'A window is open at the login page. Log in there, then press "I am logged in". The AI starts after that.'

// The person pressed Cancel while the run waited for the login: not a failure, nothing is counted.
export class LoginWaitCancelled extends Error {
  constructor() {
    super('The login was cancelled.')
    this.name = 'LoginWaitCancelled'
  }
}

const OPEN_URL = '/api/v1/tools/browser/proxy/login-window/open'
const CANCEL_URL = '/api/v1/tools/browser/proxy/login-window/cancel'

// aiLoginOpenBody: what opens the window for one AI conversation. The window's id IS the conversation's id, and aiMayUse says the AI may read
// pages of this site in it once the person has confirmed (the Browser tool checks the site and the confirmation).
export function aiLoginOpenBody(
  conversationId: string,
  link: { url: string; loginUrl?: string },
): { requestId: string; url: string; loginUrl: string; aiMayUse: true } {
  return { requestId: conversationId, url: link.url, loginUrl: (link.loginUrl ?? '').trim(), aiMayUse: true }
}

// openErrorText turns the Browser's refusal to open the window into one sentence for the person.
export function openErrorText(status: number, body: string): string {
  const text = body.trim()
  if (status === 409 && text.includes('already open')) {
    return 'A login window for this site is still open from earlier. Finish or close it, then press Generate again.'
  }
  if (status === 502) return 'The login window could not be opened (is Chrome installed? see Browser & Engine).'
  if (text) return text
  return `Could not open the login window (${status.toString()}).`
}

// openAiLoginWindow opens the window for a new AI conversation. Throws an Error with the reason when the Browser refuses.
export async function openAiLoginWindow(conversationId: string, link: { url: string; loginUrl?: string }): Promise<void> {
  const res = await fetch(OPEN_URL, {
    method: 'POST',
    credentials: 'include',
    body: JSON.stringify(aiLoginOpenBody(conversationId, link)),
  })
  if (res.ok) return
  const text = await res.text().catch(() => '')
  throw new Error(openErrorText(res.status, text))
}

// closeLoginWindow closes a window (and so throws away everything in it). Best effort: it never throws; a window that is gone is fine.
export async function closeLoginWindow(requestId: string): Promise<void> {
  try {
    await fetch(CANCEL_URL, { method: 'POST', credentials: 'include', body: JSON.stringify({ requestId }) })
  } catch {
    // nothing to do: the Browser tool closes the window by its own time limits
  }
}

// mayStartCrawlByItself: false for a link that needs a login. The run follower never starts a crawl from a request flag for it.
export function mayStartCrawlByItself(link: { requiresLogin?: boolean }): boolean {
  return link.requiresLogin !== true
}

// loginAddressForSave: the login page to send, trimmed; '' (the link's own address is opened) when the link does not need a login.
export function loginAddressForSave(requiresLogin: boolean, raw: string): string {
  return requiresLogin ? raw.trim() : ''
}

// confirmLogin asks the Browser tool to check the page in the run's window and mark the login as confirmed (without keeping anything).
// Throws an Error with the reason when the Browser refuses (the person is told to finish logging in). Keeping the login is
// confirmLoginKeeping in loginSession.ts.
export async function confirmLogin(runId: string): Promise<void> {
  await confirmLoginKeeping(runId, false)
}
