import { apiDelete, apiGet, apiPatch, apiPost } from './client'

// Already camelCase on the wire (api/src/conversation/handler's own
// response DTOs) — no snake_case *Raw mapping needed, same as
// api/platforms.ts.

// platformId/model are fixed at creation and never change afterward
// (plan/ai/conversation/step-07-fixed-platform-and-model-per-conversation.md).
//
// activeTurn (plan/ai/conversation/step-26-list-endpoint-active-turn-summary.md)
// is populated only by the list endpoint (fetchConversations below) —
// absent from create/rename results, whose own backend handlers never
// set it (see that step's own reasoning for keeping it off the shared
// response DTO those share).
export interface Conversation {
  id: string
  title: string
  startedAt: string
  platformId: string
  model: string
  activeTurn?: ActiveTurnSummary
}

// No `id` field on a message — content lives in a file, not a database
// row (plan/ai/conversation/step-01); a message is identified by its
// own timestamp instead.
//
// failed/error surface a send that failed (plan/ai/conversation/step-08) —
// undefined for a normal message. A failed message is always role
// "user" with no paired assistant message, since there never was one.
//
// durationMs (plan/ai/conversation/step-14) is how long this turn took
// to resolve — present on the "assistant" entry of a successful turn
// and the "user" entry of a failed one, undefined on a plain
// successful "user" entry (which has no "how long did it take" of its
// own). Raw milliseconds from the backend; formatting is this
// frontend's own job (step-15).
export interface ConversationMessage {
  role: 'user' | 'assistant'
  content: string
  createdAt: string
  failed?: boolean
  error?: string
  durationMs?: number
}

// TurnRunStatus mirrors the backend's own turn_runs.status CHECK
// constraint exactly (plan/ai/conversation/step-23-detach-turn-execution-from-request.md).
export type TurnRunStatus = 'running' | 'completed' | 'failed' | 'cancelled'

// ActiveTurnSummary is the lean projection of a turn's own execution
// state — startedAt is the server's own timestamp, captured once at
// turn-start, so a reopened page can compute real elapsed time instead
// of restarting a local timer from zero
// (plan/ai/conversation/step-24-server-tracked-turn-elapsed-time.md
// builds directly on this field). serverNow is this response's own
// clock at response time — never stored, always freshly computed —
// letting a client correct for clock skew: elapsed = (Date.now() +
// (serverNow - Date.now())) - startedAt. This is what
// fetchConversations' own per-item `activeTurn` field
// (plan/ai/conversation/step-26-list-endpoint-active-turn-summary.md)
// carries — cheap enough to include for every conversation in a list
// that's polled every few seconds
// (plan/ai/conversation/step-27-frontend-periodic-list-refresh.md).
export interface ActiveTurnSummary {
  turnRunId: string
  status: TurnRunStatus
  startedAt: string
  serverNow: string
}

// ActiveTurn is the fuller shape fetchActiveTurn (below) and
// fetchConversation's own `activeTurn` field return — everything
// ActiveTurnSummary has, plus the message that started this run (not
// yet in `messages` — the conversation's own Markdown log only gains
// this turn once it finishes, so a page reopened mid-turn needs this
// to show it) and a coarse, step-by-step trace of the tool-calling
// loop's own progress — never raw model/tool output.
export interface ActiveTurn extends ActiveTurnSummary {
  userContent: string
  log: string[]
}

export interface ConversationDetail extends Conversation {
  messages: ConversationMessage[]
  // Present only while a turn is actually still running for this
  // conversation right now — absent once it finishes (unlike
  // fetchActiveTurn below, which keeps reporting the last run's
  // terminal status).
  activeTurn?: ActiveTurn
}

// StartedTurnRun — what POST .../messages now returns (plan/ai/conversation/step-23):
// this route no longer waits for the AI to finish; it starts a
// detached run and returns immediately. Callers must poll
// fetchActiveTurn until its status leaves "running", then call
// fetchConversation for the real, finished message.
export interface StartedTurnRun {
  turnRunId: string
  status: TurnRunStatus
  startedAt: string
}

export async function fetchConversations(): Promise<Conversation[]> {
  const raw = await apiGet<{ message: Conversation[] }>('/api/v1/conversations')
  return raw.message
}

export async function createConversation(input: { title?: string; platformId: string; model?: string }): Promise<Conversation> {
  const raw = await apiPost<{ message: Conversation }>('/api/v1/conversations', input)
  return raw.message
}

export async function fetchConversation(id: string): Promise<ConversationDetail> {
  const raw = await apiGet<{ message: ConversationDetail }>(`/api/v1/conversations/${encodeURIComponent(id)}`)
  return raw.message
}

export async function renameConversation(id: string, title: string): Promise<Conversation> {
  const raw = await apiPatch<{ message: Conversation }>(`/api/v1/conversations/${encodeURIComponent(id)}`, { title })
  return raw.message
}

export async function deleteConversation(id: string): Promise<void> {
  await apiDelete(`/api/v1/conversations/${encodeURIComponent(id)}`)
}

// platformId/model no longer travel per-message — a conversation's own
// fixed values apply automatically (step 7). Returns as soon as the
// turn run is started — NOT once the AI has replied (step 23). The
// caller is responsible for polling fetchActiveTurn until it leaves
// "running", then calling fetchConversation for the real message.
export async function sendMessage(conversationId: string, input: { content: string }): Promise<StartedTurnRun> {
  const raw = await apiPost<{ message: StartedTurnRun }>(
    `/api/v1/conversations/${encodeURIComponent(conversationId)}/messages`,
    input,
  )
  return raw.message
}

// GET .../turns/active — the conversation's own most recent turn run,
// regardless of status (deliberately not restricted to "running": a
// poll racing the exact moment a run finishes must still observe the
// real terminal status instead of a 404 it could misread as "the run
// vanished"). Throws (via apiGet's own ApiError) with status 404 only
// when no turn has ever been started for this conversation.
export async function fetchActiveTurn(conversationId: string): Promise<ActiveTurn> {
  const raw = await apiGet<{ message: ActiveTurn }>(`/api/v1/conversations/${encodeURIComponent(conversationId)}/turns/active`)
  return raw.message
}

// POST .../turns/active/stop — requests cancellation of the
// conversation's own currently-running turn (plan/ai/conversation/step-25-cancel-in-progress-turn.md).
// Cancellation is asynchronous: this resolves once the stop signal is
// sent, not once the run has actually stopped — keep polling
// fetchActiveTurn until its status leaves "running" (it becomes
// "cancelled", not "failed").
export async function stopActiveTurn(conversationId: string): Promise<void> {
  await apiPost<{ message: { status: string } }>(`/api/v1/conversations/${encodeURIComponent(conversationId)}/turns/active/stop`)
}
