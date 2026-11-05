import type { PortalLink } from '../api'
import { mayStartCrawlByItself } from './loginWindow'

// The rules of the run follower (plan/ai/tools/career/step-163-job-detail-instructions-pipeline.md, step 4): given one link, what must happen
// next that no page needs to be open for. Pure: no React, no network. Starting a NEW AI run is not here (it needs the page's AI choice).
export type RunKind = 'listing' | 'job_detail'

export type FollowAction =
  { kind: 'listing_crawl' } | { kind: 'job_detail_crawl' } | { kind: 'resume_run'; runKind: RunKind; conversationId: string }

export interface FollowContext {
  // True while a Platforms page of this tab is itself carrying a run of the link (its own closure finishes it, so the follower stays out).
  pageBusy: (linkId: string) => boolean
  // True when the follower already follows this conversation.
  followed: (conversationId: string) => boolean
}

type FollowLink = Pick<
  PortalLink,
  | 'id'
  | 'crawlInstructions'
  | 'jobDetailCrawlInstructions'
  | 'listingCrawlRequested'
  | 'jobDetailCrawlRequested'
  | 'requiresLogin'
  | 'hasActiveCrawlRun'
  | 'instructionsAiConversationId'
  | 'jobDetailInstructionsAiConversationId'
  | 'jobsQuestion'
  | 'instructionStats'
>

function has(text: string | null | undefined): boolean {
  return text !== undefined && text !== null && text.trim() !== ''
}

export function followActions(link: FollowLink, ctx: FollowContext): FollowAction[] {
  if (ctx.pageBusy(link.id)) return []
  const actions: FollowAction[] = []
  const listingConversation = link.instructionsAiConversationId ?? ''
  const detailConversation = link.jobDetailInstructionsAiConversationId ?? ''
  // A listing run that ended on the AI's question is waiting for the user, not running.
  const waitingForAnswer = link.jobsQuestion !== undefined

  // A run whose turn nobody follows (the page was closed or reloaded): finish its bookkeeping.
  if (has(listingConversation) && !waitingForAnswer && !ctx.followed(listingConversation)) {
    actions.push({ kind: 'resume_run', runKind: 'listing', conversationId: listingConversation })
  }
  if (has(detailConversation) && detailConversation !== listingConversation && !ctx.followed(detailConversation)) {
    actions.push({ kind: 'resume_run', runKind: 'job_detail', conversationId: detailConversation })
  }

  // The AI and a crawl share the one browser: no crawl starts while a run of the link goes on; the flag stays set and the crawl starts when
  // the run is over.
  const aiRunning =
    (has(listingConversation) && !waitingForAnswer) ||
    has(detailConversation) ||
    link.instructionStats.jobDetail.running ||
    (link.instructionStats.listing.running && !waitingForAnswer)
  if (aiRunning || link.hasActiveCrawlRun) return actions
  // A link that needs a login is crawled by hand only: a request flag never starts a crawl for it (plan 182).
  if (!mayStartCrawlByItself(link)) return actions

  // One crawl at a time (the server refuses a second): the listing first.
  if (link.listingCrawlRequested && has(link.crawlInstructions)) {
    actions.push({ kind: 'listing_crawl' })
  } else if (link.jobDetailCrawlRequested && has(link.jobDetailCrawlInstructions)) {
    actions.push({ kind: 'job_detail_crawl' })
  }
  return actions
}
