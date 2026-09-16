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
export async function createConversation(input: { title?: string; platformId: string; model?: string; hidden?: boolean }): Promise<Conversation> {
  const res = await httpClient.post('/api/v1/conversations', input)
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

export async function sendMessage(conversationId: string, content: string): Promise<SendMessageResult> {
  const res = await httpClient.post(`/api/v1/conversations/${encodeURIComponent(conversationId)}/messages`, { content })

  await httpClient.responseBody<StartedTurnRun>(res, 'send message')

  for (; ;) {
    const turn = await fetchActiveTurn(conversationId)
    if (turn.status !== 'running') break
    await new Promise((resolve) => setTimeout(resolve, TURN_POLL_INTERVAL_MS))
  }

  const detail = await fetchConversationDetail(conversationId)
  const last = detail.messages[detail.messages.length - 1]
  if (!last) {
    throw new CoreApiError(500, 'the AI turn finished but produced no message')
  }
  if (last.failed) {
    throw new CoreApiError(502, last.error || 'the AI turn failed')
  }

  return { role: last.role, content: last.content, createdAt: last.createdAt, durationMs: last.durationMs }
}
