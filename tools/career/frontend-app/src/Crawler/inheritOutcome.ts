// What the Portals page says, and does, about "a new link first tries the portal's
// existing instructions" (step 103). Pure (no React, no fetching) so it can be
// script-checked. The response is the backend's POST /portal-links/inherit-instructions;
// every field a record might lack is optional here on purpose.

export interface InheritTried {
  linkId?: string
  title?: string
  // ok | failed | error
  listing?: string
  // ok | failed | no-instructions | not-tried (only after a list that worked)
  jobDetail?: string
  reason?: string
}

export interface InheritResponse {
  inherited?: boolean
  listing?: boolean
  jobDetail?: boolean
  from?: { linkId?: string; title?: string } | null
  tried?: InheritTried[] | null
}

// success: the listing was copied (kind partial when the details were not);
// none: nothing was copied, there was something to say or nothing at all;
// error: the trial itself could not run.
export type InheritKind = 'success' | 'partial' | 'none' | 'error'

export interface InheritView {
  // Empty when there is nothing to tell (the portal had no other instructions).
  message: string
  kind: InheritKind
  // True when the AI run for the listing instructions must not start: they were inherited.
  skipAi: boolean
}

// Only a link with no listing instructions can inherit (the backend refuses any other).
export function shouldTryInherit(link: { crawlInstructions?: string | null }): boolean {
  return (link.crawlInstructions ?? '').trim() === ''
}

function nameOf(title: string | undefined): string {
  const t = (title ?? '').trim()
  return t === '' ? '(untitled link)' : t
}

const MAX_LISTED = 3

export function inheritView(res: InheritResponse): InheritView {
  const tried = res.tried ?? []
  if (res.inherited === true && res.listing === true) {
    const from = nameOf(res.from?.title)
    if (res.jobDetail === true) {
      return {
        message: `Used the crawl and job-detail instructions of "${from}": they worked on this page.`,
        kind: 'success',
        skipAi: true,
      }
    }
    return {
      message: `Used the crawl instructions of "${from}": they worked on this page. The job-detail instructions are prepared once its jobs are crawled.`,
      kind: 'partial',
      skipAi: true,
    }
  }
  if (tried.length === 0) return { message: '', kind: 'none', skipAi: false }

  const failedToRun = tried.find((t) => t.listing === 'error')
  if (failedToRun) {
    return {
      message: `Could not try the portal's other instructions${failedToRun.reason ? `: ${failedToRun.reason}` : ''}.`,
      kind: 'error',
      skipAi: false,
    }
  }
  const parts = tried.slice(0, MAX_LISTED).map((t) => `${nameOf(t.title)}: ${(t.reason ?? '').trim() || 'did not work'}`)
  const more = tried.length > MAX_LISTED ? ` and ${tried.length - MAX_LISTED} more` : ''
  return {
    message: `The instructions of this portal's other links did not work on this page (${parts.join('; ')}${more}).`,
    kind: 'none',
    skipAi: false,
  }
}

// A failed request is never a reason to block the AI run: it only says so.
export function inheritErrorView(err: unknown): InheritView {
  const text = err instanceof Error ? err.message.trim() : String(err).trim()
  return {
    message: `Could not try the portal's other instructions${text ? `: ${text}` : ''}.`,
    kind: 'error',
    skipAi: false,
  }
}
