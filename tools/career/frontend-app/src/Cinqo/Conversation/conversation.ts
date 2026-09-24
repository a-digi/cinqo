// conversation.ts — the Conversation slice of cinqo's own CORE API
// (/api/v1/conversations), split out of coreApi.ts (which keeps the
// separate Platform slice). Shared HTTP/JSON client plumbing
// (CoreApiError, and httpClient's own isSuccess/responseBody methods)
// lives in Cinqo/Http/client.ts, used by both slices. Same deal as
// coreApi.ts's own top comment: this tool's bundle runs as plain JS in
// the same page/origin as the rest of the app, so the same session
// cookie already authenticates these calls, and each route below
// already enforces its own scope independently — this file adds a new
// CALLER, not a new capability. See
// plan/ai/tools/career/step-24-manual-crawl-trigger.md.
import { CoreApiError, httpClient } from '../Http/client'

export interface Conversation {
  id: string
  title: string
  startedAt: string
  platformId: string
  model: string
}

// hidden (step 33, plan/ai/conversation/step-30-hidden-conversations.md)
// — omitted (or false) preserves today's exact behavior; true excludes
// this conversation from the normal GET /api/v1/conversations list.
// Used by the "generate crawl instructions with AI" flow
// (generateInstructions.ts) — never by "Crawl with AI" (crawl.ts),
// which stays visible/reviewable.
export async function createConversation(input: {
  title?: string
  platformId: string
  model?: string
  hidden?: boolean
}): Promise<Conversation> {
  // toolSlug is always "career" here, never caller-supplied — every
  // conversation this function creates (Job Match, Generate CV PDF,
  // crawl instructions) IS a Career conversation, and centralizing it
  // in this one shared wrapper means every current and future call
  // site is tagged automatically, with nothing to remember at each one.
  // Lets the core /conversations page filter "by tool" (step 41). See
  // plan/ai/conversation/step-41-search-and-filter-by-tool.md.
  const res = await httpClient.post('/api/v1/conversations', { ...input, toolSlug: 'career' })
  return httpClient.responseBody<Conversation>(res, 'create conversation')
}

export interface SendMessageResult {
  role: string
  content: string
  createdAt: string
  durationMs?: number
}

// TurnRunStatus/ActiveTurn/StartedTurnRun mirror the primary admin
// frontend's own frontend/src/api/conversations.ts types exactly — see
// plan/ai/conversation/step-23-detach-turn-execution-from-request.md.
// POST .../messages no longer waits for the AI (this tool is the
// second, independent production caller of that same route — see that
// step's own Dependencies section) — it now only starts a detached
// turn run; sendMessage below polls .../turns/active until it finishes
// and returns the same SendMessageResult shape callers already expect,
// so CrawlPanel.tsx's own handleCrawlWithAI needs no further change.
export type TurnRunStatus = 'running' | 'completed' | 'failed' | 'cancelled'

interface ActiveTurn {
  turnRunId: string
  status: TurnRunStatus
  startedAt: string
  userContent: string
  log: string[]
}

interface StartedTurnRun {
  turnRunId: string
  status: TurnRunStatus
  startedAt: string
}

interface ConversationMessage {
  role: 'user' | 'assistant'
  content: string
  createdAt: string
  failed?: boolean
  error?: string
  durationMs?: number
}

interface ConversationDetail {
  id: string
  messages: ConversationMessage[]
  activeTurn?: ActiveTurn
}

const TURN_POLL_INTERVAL_MS = 2000

async function fetchActiveTurn(conversationId: string): Promise<ActiveTurn> {
  const res = await httpClient.get(`/api/v1/conversations/${encodeURIComponent(conversationId)}/turns/active`)
  return httpClient.responseBody<ActiveTurn>(res, 'load active turn')
}

async function fetchConversationDetail(conversationId: string): Promise<ConversationDetail> {
  const res = await httpClient.get(`/api/v1/conversations/${encodeURIComponent(conversationId)}`)
  return httpClient.responseBody<ConversationDetail>(res, 'load conversation')
}

// awaitTurnCompletion polls .../turns/active until the conversation's
// most recent turn leaves "running", then reads back the final
// message — the shared tail end both a fresh send (sendMessage below)
// and a resumed one (CrawlPanel.tsx's own "Check Progress - AI" path,
// picking a still-running generate-instructions run back up after a
// reload) need. See
// plan/ai/tools/career/step-60-generate-with-ai-live-chat-window.md.
export async function awaitTurnCompletion(conversationId: string): Promise<SendMessageResult> {
  for (;;) {
    const turn = await fetchActiveTurn(conversationId)
    if (turn.status !== 'running') break
    await new Promise((resolve) => setTimeout(resolve, TURN_POLL_INTERVAL_MS))
  }

  const detail = await fetchConversationDetail(conversationId)
  const last = detail.messages[detail.messages.length - 1]
  // noUncheckedIndexedAccess isn't on, so TS types this indexed access as
  // always-defined — it genuinely isn't when messages is empty.
  // eslint-disable-next-line @typescript-eslint/no-unnecessary-condition
  if (!last) {
    throw new CoreApiError(500, 'the AI turn finished but produced no message')
  }
  if (last.failed) {
    throw new CoreApiError(502, last.error ?? 'the AI turn failed')
  }

  return { role: last.role, content: last.content, createdAt: last.createdAt, durationMs: last.durationMs }
}

export async function sendMessage(conversationId: string, content: string): Promise<SendMessageResult> {
  const res = await httpClient.post(`/api/v1/conversations/${encodeURIComponent(conversationId)}/messages`, { content })
  await httpClient.responseBody<StartedTurnRun>(res, 'send message')
  return awaitTurnCompletion(conversationId)
}

// fetchTurnStatus is the resume path's own "is it still going" check
// (CrawlPanel.tsx, on mount) — null means no turn has ever been
// started for this conversation id (a 404 from .../turns/active),
// treated as "not running," never as an error to surface.
export async function fetchTurnStatus(conversationId: string): Promise<TurnRunStatus | null> {
  try {
    const turn = await fetchActiveTurn(conversationId)
    return turn.status
  } catch (err) {
    if (err instanceof CoreApiError && err.status === 404) {
      return null
    }
    throw err
  }
}
