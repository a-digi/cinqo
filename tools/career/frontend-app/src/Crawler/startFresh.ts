// startFresh.ts (step 161) — the rules behind a link's "Start fresh" button, kept free of React so they can be
// script-checked: when it is blocked, what the dialog says, how the clean-up runs and how its result reads.
// See plan/ai/tools/career/step-161-start-fresh-for-a-link.md.
import type { PortalLink, StartFreshResult } from '../api'

export interface StartFreshDeps {
  startFresh: (portalLinkId: string, clearInstructions: boolean) => Promise<StartFreshResult>
  // 'gone' (the conversation no longer exists) counts as deleted; a throw counts as failed.
  deleteConversation: (conversationId: string) => Promise<'deleted' | 'gone'>
}

export interface StartFreshOutcome {
  attemptsRemoved: number
  conversationsFound: number
  conversationsDeleted: number
  conversationsFailed: number
}

// runStartFresh resets the link on the server first, then deletes the conversations it reported, one by one: a
// conversation that cannot be deleted (it belongs to someone else, a failure) is counted and never stops the rest.
// A server refusal (a run is going) throws before anything is deleted.
export async function runStartFresh(deps: StartFreshDeps, portalLinkId: string, clearInstructions: boolean): Promise<StartFreshOutcome> {
  const result = await deps.startFresh(portalLinkId, clearInstructions)
  let deleted = 0
  let failed = 0
  for (const id of result.conversationIds) {
    try {
      await deps.deleteConversation(id)
      deleted++
    } catch {
      failed++
    }
  }
  return {
    attemptsRemoved: result.attemptsRemoved,
    conversationsFound: result.conversationIds.length,
    conversationsDeleted: deleted,
    conversationsFailed: failed,
  }
}

const plural = (n: number, one: string, many: string) => `${n} ${n === 1 ? one : many}`

// startFreshBlockedReason says why the button is disabled, or null when it can be used. A generation (listing or
// job details) running for the link — this tab's own, or one the server knows about — blocks it, and so does the
// trial of the platform's other instructions.
export function startFreshBlockedReason(
  link: Pick<PortalLink, 'instructionStats'>,
  tabBusy: { listingGenerating: boolean; jobDetailGenerating: boolean; inheriting: boolean },
): string | null {
  const running =
    tabBusy.listingGenerating ||
    tabBusy.jobDetailGenerating ||
    link.instructionStats.listing.running ||
    link.instructionStats.jobDetail.running
  if (running) return 'An AI generation is running for this link. Stop it or wait until it has finished.'
  if (tabBusy.inheriting) return "Trying the instructions of this platform's other links first. Wait until it has finished."
  return null
}

// startFreshAttemptsRecorded is how many finished attempts the link shows (both kinds).
export function startFreshAttemptsRecorded(link: Pick<PortalLink, 'instructionStats'>): number {
  return link.instructionStats.listing.attempts + link.instructionStats.jobDetail.attempts
}

export const START_FRESH_CLEAR_INSTRUCTIONS_LABEL = 'Also remove the stored crawl instructions (job list and job details)'
export const START_FRESH_GENERATE_LABEL = 'Then generate the crawl instructions again with a new AI conversation'

// startFreshExplanation is the dialog text.
export function startFreshExplanation(link: Pick<PortalLink, 'instructionStats'>): string {
  const n = startFreshAttemptsRecorded(link)
  const recorded =
    n === 0 ? 'No finished attempts are recorded for this link.' : `${plural(n, 'attempt is', 'attempts are')} recorded for this link.`
  return (
    "This deletes the AI conversations of this link's earlier attempts, with their prompt and log files, and clears the attempt history, " +
    `so the attempt counts and “Probably can't be crawled” go away. ${recorded} Conversations that cannot be deleted are reported.`
  )
}

// startFreshResultLine is what is shown once it ran.
export function startFreshResultLine(o: StartFreshOutcome): string {
  const parts = [`Cleared ${plural(o.attemptsRemoved, 'attempt', 'attempts')}`]
  parts.push(
    o.conversationsFound === 0
      ? 'no earlier conversations were found'
      : `deleted ${plural(o.conversationsDeleted, 'conversation', 'conversations')}`,
  )
  let line = `${parts.join(' and ')}.`
  if (o.conversationsFailed > 0) {
    line += ` ${plural(o.conversationsFailed, 'conversation', 'conversations')} could not be deleted (they may belong to someone else); you can delete them in Conversations.`
  }
  return line
}
