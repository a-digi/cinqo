import { apiDelete, apiGet, apiPatch, apiPost, apiPut } from './client'

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
  // promptTokens/completionTokens (step 35) — the real, provider-
  // reported token usage this turn accumulated, same placement as
  // durationMs (set on the assistant entry of a successful turn and
  // the user entry of a failed one). Absent for a turn logged before
  // this field existed, or one whose provider never reported usage.
  promptTokens?: number
  completionTokens?: number
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
  // promptTokens/completionTokens/totalTokens (step 34) are the real,
  // provider-reported token counts accumulated so far this turn —
  // already correct and live-updating on every poll while the turn is
  // still "running", not only once it finishes. See
  // plan/ai/conversation/step-34-realtime-token-usage-budget-and-display.md.
  promptTokens: number
  completionTokens: number
  totalTokens: number
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
  const raw = await apiPost<{ message: StartedTurnRun }>(`/api/v1/conversations/${encodeURIComponent(conversationId)}/messages`, input)
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

// AiTraceLogSummary is one turn's own trace file, listed without ever
// reading its content (plan/ai/conversation/step-36-ai-trace-logs.md)
// — a diagnostic record of the full raw request/response exchange with
// the AI model, for understanding excessive token usage. The frontend
// deliberately never fetches/renders a file's own content — only where
// it lives (AiTraceLogsResult.folder) and how many exist; the backend
// still supports fetching one file's raw content directly (?turnId=)
// for anyone reading it outside this UI (e.g. a text editor, curl).
export interface AiTraceLogSummary {
  turnRunId: string
  sizeBytes: number
  modifiedAt: string
}

export interface AiTraceLogsResult {
  folder: string
  logs: AiTraceLogSummary[]
}

export async function fetchAiTraceLogs(conversationId: string): Promise<AiTraceLogsResult> {
  const raw = await apiGet<{ message: AiTraceLogsResult }>(`/api/v1/conversations/${encodeURIComponent(conversationId)}/logs`)
  return raw.message
}

// AllAiTraceLogsEntry is one conversation's own trace-log summary in
// the admin-wide overview (plan/ai/conversation/step-38-ai-logs-overview-page.md)
// — every conversation that has ever produced at least one AI trace
// log, not just the caller's own (unlike fetchAiTraceLogs above).
// title is "" when that conversation has since been deleted but its
// own trace folder wasn't.
export interface AllAiTraceLogsEntry {
  conversationId: string
  title?: string
  folder: string
  logCount: number
  modifiedAt: string
}

export async function fetchAllAiTraceLogs(): Promise<AllAiTraceLogsEntry[]> {
  const raw = await apiGet<{ message: { conversations: AllAiTraceLogsEntry[] } }>('/api/v1/conversations/logs')
  return raw.message.conversations
}

// ConversationSettings (step 37) — the conversation feature's own
// single, global settings object, admin-only (unlike every other
// endpoint in this file): turning aiTraceLogsEnabled on captures raw
// model I/O for every user's conversations, not just the caller's own.
export interface ConversationSettings {
  aiTraceLogsEnabled: boolean
}

export async function fetchConversationSettings(): Promise<ConversationSettings> {
  const raw = await apiGet<{ message: ConversationSettings }>('/api/v1/conversations/settings')
  return raw.message
}

export async function updateConversationSettings(settings: ConversationSettings): Promise<ConversationSettings> {
  const raw = await apiPut<{ message: ConversationSettings }>('/api/v1/conversations/settings', settings)
  return raw.message
}
