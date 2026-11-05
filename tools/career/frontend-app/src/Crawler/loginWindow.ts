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
