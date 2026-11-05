// loginWindow.ts (plan 182, step 5) — what the Career page needs for a crawl that waits for a login: when a run is waiting, what to tell the
// person, and the one call that confirms the login. The person logs in on the site's own page in the window the Browser tool opened; this file
// never sees or sends a password, a cookie or anything typed there, and keeps nothing: the confirmation is only the run's id. The run itself is
// stopped with the existing stopCrawlNow (the run closes its window).

import type { CrawlRun } from './crawlNow'

export const WAITING_FOR_LOGIN = 'waiting_for_login'

const CONFIRM_URL = '/api/v1/tools/browser/proxy/login-window/confirm'

// isWaitingForLogin: a run that is still going and waits for the person to log in and confirm.
export function isWaitingForLogin(run: Pick<CrawlRun, 'status' | 'phase'> | null): boolean {
  return run !== null && run.status === 'running' && run.phase === WAITING_FOR_LOGIN
}

export const LOGIN_BADGE = 'Login needed'
export const LOGIN_BADGE_TOOLTIP =
  'This page needs you to log in. Every crawl opens a window for you to log in on the site itself, so it is never crawled automatically.'

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

export const NO_AI_FOR_LOGIN_NOTE =
  'The AI cannot read pages behind your login yet, so Generate is off for this link. Write its crawl instructions by hand: open Job list (and Job details) under Crawl instructions.'

// mayGenerateWithAI: false for a link that needs a login. The AI's browser tools read the logged-out page, so a run would only fail, cost
// tokens and count as a failed attempt. The server refuses it as well (it does not rely on this page).
export function mayGenerateWithAI(link: { requiresLogin?: boolean }): boolean {
  return link.requiresLogin !== true
}

// generateBlockedNote: the reason to show where Generate is off for a link that needs a login, or null for any other link.
export function generateBlockedNote(link: { requiresLogin?: boolean }): string | null {
  return mayGenerateWithAI(link) ? null : NO_AI_FOR_LOGIN_NOTE
}

// mayStartCrawlByItself: false for a link that needs a login. The run follower never starts a crawl from a request flag for it.
export function mayStartCrawlByItself(link: { requiresLogin?: boolean }): boolean {
  return link.requiresLogin !== true
}

// loginAddressForSave: the login page to send, trimmed; '' (the link's own address is opened) when the link does not need a login.
export function loginAddressForSave(requiresLogin: boolean, raw: string): string {
  return requiresLogin ? raw.trim() : ''
}

// confirmErrorText turns the Browser's refusal of a confirmation into one sentence for the person. The Browser's own text names the reason
// (the page still shows a login form, it is a bot check, the page is empty); a 404 means the window is gone.
export function confirmErrorText(status: number, body: string): string {
  if (status === 404) return 'The login window is closed. Start the crawl again to get a new one.'
  const text = body.trim()
  if (text) return text
  return `Could not confirm the login (${status.toString()}).`
}

// confirmLogin asks the Browser tool to check the page in the run's window and mark the login as confirmed. Throws an Error with the reason
// when the Browser refuses (the person is told to finish logging in).
export async function confirmLogin(runId: string): Promise<void> {
  const res = await fetch(CONFIRM_URL, {
    method: 'POST',
    credentials: 'include',
    body: JSON.stringify({ requestId: runId }),
  })
  if (res.ok) return
  const text = await res.text().catch(() => '')
  throw new Error(confirmErrorText(res.status, text))
}
