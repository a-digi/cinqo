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
  // The tools the conversation is restricted to; missing when it is not restricted. Echoed by the
  // server (step 113) so a caller that asked for a restriction can check it took effect.
  allowedTools?: string[]
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
  // toolIterationLimit (step 93) lowers how many model calls one turn may make
  // (1 to 15; omitted means the server default of 15). Only lowers, never raises.
  toolIterationLimit?: number
  // allowedTools (step 113) restricts the conversation to these MCP tools: only they are offered
  // to the model and any other call is refused. Omitted means no restriction.
  allowedTools?: string[]
  // compactToolResults (plan/ai/conversation/step-45, step 3) opts the conversation into three rules of the tool loop that only change
  // what the model is shown: old bulky tool results are folded into one-line stubs, an identical repeated read-only call is answered in
  // one line, and a note warns two rounds before the budget ends. A core that does not know the field ignores it.
  compactToolResults?: boolean
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
  // How many tool calls of the turn that produced this message the core refused or rejected; absent from an older core.
  toolErrors?: number
  // The names of the tools the turn called, in order (read from the turn's log); undefined when the log says nothing (an older core).
  toolsCalled?: string[]
}

// The tools a turn called, from its log: the core writes one line "<time>\titeration <n>: invoking tool <name>" for each call it makes.
// undefined when the log has no line about the model at all (an older core, or a log that was not kept), so "no call" is never guessed.
export function toolsCalledOf(log: string[] | undefined): string[] | undefined {
  if (!log?.some((line) => line.includes('calling model'))) return undefined
  const names: string[] = []
  for (const line of log) {
    const m = /invoking tool (\S+)/.exec(line)
    if (m?.[1]) names.push(m[1])
  }
  return names
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
  // How many tool calls of the turn the core refused or rejected (a tool that was not available, arguments that failed the schema).
  // Absent from an older core.
  toolErrors?: number
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
  let turn = await fetchActiveTurn(conversationId)
  while (turn.status === 'running') {
    await new Promise((resolve) => setTimeout(resolve, TURN_POLL_INTERVAL_MS))
    turn = await fetchActiveTurn(conversationId)
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

  return {
    role: last.role,
    content: last.content,
    createdAt: last.createdAt,
    durationMs: last.durationMs,
    toolErrors: turn.toolErrors,
    toolsCalled: toolsCalledOf(turn.log),
  }
}

// The settings of one message that differ from the conversation's own (core, plan 210): fresh sends the model this message only (none of the earlier turns,
// which stay in the conversation for whoever reads it), allowedTools narrows the tools this turn is offered to those of the conversation's own list it names.
// Omitted, the message is an ordinary one.
export interface TurnSettings {
  fresh?: boolean
  allowedTools?: readonly string[]
}

// The request body of a message: the text, and only the settings that are set (an ordinary message is just {content}, as it always was).
export function messageBody(content: string, settings?: TurnSettings): { content: string; fresh?: boolean; allowedTools?: readonly string[] } {
  const body: { content: string; fresh?: boolean; allowedTools?: readonly string[] } = { content }
  if (settings?.fresh === true) body.fresh = true
  if (settings?.allowedTools !== undefined) body.allowedTools = settings.allowedTools
  return body
}

export async function sendMessage(conversationId: string, content: string, settings?: TurnSettings): Promise<SendMessageResult> {
  const res = await httpClient.post(`/api/v1/conversations/${encodeURIComponent(conversationId)}/messages`, messageBody(content, settings))
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

// deleteConversation removes one conversation with its prompt/answer file and AI logs (the core's own
// DELETE /api/v1/conversations/{id}, owner only). 'gone' means it did not exist (404): already deleted, which
// is what the caller wanted. Used by "Start fresh" (step 161) for the conversations of a link's earlier AI runs.
export async function deleteConversation(conversationId: string): Promise<'deleted' | 'gone'> {
  const res = await httpClient.delete(`/api/v1/conversations/${encodeURIComponent(conversationId)}`)
  if (res.status === 404) return 'gone'
  if (!httpClient.isSuccess(res)) {
    const body = (await res.json().catch(() => undefined)) as { message?: unknown } | undefined
    throw new CoreApiError(
      res.status,
      body && typeof body.message === 'string' ? body.message : `failed to delete conversation (${res.status})`,
    )
  }
  return 'deleted'
}
