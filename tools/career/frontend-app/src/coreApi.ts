// coreApi.ts — thin fetch helpers for cinqo's own CORE endpoints
// (/api/v1/platforms, /api/v1/conversations), as opposed to api.ts's
// own PROXY_BASE calls to this tool's own backend. This tool's bundle
// runs as plain JS in the same page/origin as the rest of the app, so
// the same session cookie already authenticates these calls, and each
// route below already enforces its own scope independently — this
// file adds a new CALLER, not a new capability. See
// plan/ai/tools/career/step-24-manual-crawl-trigger.md.
//
// No 401-retry/session-renewal here (unlike the core app's own
// api/client.ts) — deliberately simple, matching this tool's own
// api.ts convention; a session that expires mid-crawl surfaces as a
// plain error in the crawl result line, same as any other failure.

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

async function coreJsonOrThrow<T>(res: Response, action: string): Promise<T> {
  const body = (await res.json().catch(() => undefined)) as { message?: unknown } | undefined
  if (!res.ok) {
    const message = body && typeof body.message === 'string' ? body.message : `failed to ${action} (${res.status})`
    throw new CoreApiError(res.status, message)
  }
  return (body as { message: T }).message
}

export interface Platform {
  id: string
  name: string
  models: string[]
}

export async function fetchPlatforms(): Promise<Platform[]> {
  const res = await fetch('/api/v1/platforms', { credentials: 'include' })
  return coreJsonOrThrow<Platform[]>(res, 'load AI platforms')
}

export interface Conversation {
  id: string
  title: string
  startedAt: string
  platformId: string
  model: string
}

export async function createConversation(input: { title?: string; platformId: string; model?: string }): Promise<Conversation> {
  const res = await fetch('/api/v1/conversations', {
    method: 'POST',
    credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(input),
  })
  return coreJsonOrThrow<Conversation>(res, 'create conversation')
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
// so PortalsPage.tsx's own handleCrawlWithAI needs no further change.
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
  const res = await fetch(`/api/v1/conversations/${encodeURIComponent(conversationId)}/turns/active`, { credentials: 'include' })
  return coreJsonOrThrow<ActiveTurn>(res, 'load active turn')
}

async function fetchConversationDetail(conversationId: string): Promise<ConversationDetail> {
  const res = await fetch(`/api/v1/conversations/${encodeURIComponent(conversationId)}`, { credentials: 'include' })
  return coreJsonOrThrow<ConversationDetail>(res, 'load conversation')
}

export async function sendMessage(conversationId: string, content: string): Promise<SendMessageResult> {
  const res = await fetch(`/api/v1/conversations/${encodeURIComponent(conversationId)}/messages`, {
    method: 'POST',
    credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ content }),
  })
  await coreJsonOrThrow<StartedTurnRun>(res, 'send message')

  for (;;) {
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
