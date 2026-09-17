import { useState } from 'react'
import { fetchSubAgentsForTurn, type ConversationMessage, type SubAgentRun } from '../../api/conversations'
import { ApiError } from '../../api/client'
import { Markdown } from '../../Shared/Components/Markdown/Markdown'
import { ThinkingIndicator } from './ThinkingIndicator'
import { SubAgentRow } from './SubAgentRow'
import { formatDuration } from './formatDuration'
import { formatTokenCount } from './formatTokenCount'

// Renders the active conversation's messages in order, user/assistant
// styled distinctly. No streaming — a sent message shows a
// "thinking…" placeholder (pendingUserContent/sending) until the
// single response resolves. A failed send (plan/ai/conversation/step-08)
// renders as its own distinct bubble with a resend action and an
// inline error-detail toggle — durable across a reload, not just this
// live session's own transient state. See
// plan/ai/conversation/step-05-frontend-chat-ui.md.
export function MessageThread({
  conversationId,
  messages,
  pendingUserContent,
  sending,
  turnStartedAt,
  turnClockOffsetMs,
  turnPromptTokens,
  turnCompletionTokens,
  turnTotalTokens,
  turnSubAgents,
  onResend,
}: {
  // Needed only to fetch a past message's own historical sub-agent
  // detail on demand (step 41 — Sub Agents, Bubble's own click-through)
  // — fetchSubAgentsForTurn takes both the conversation and turn id.
  conversationId: string
  messages: ConversationMessage[]
  pendingUserContent: string | null
  sending: boolean
  // Server-recorded turn-start time + this client's clock offset from
  // the server — see ThinkingIndicator's own doc comment and
  // plan/ai/conversation/step-24-server-tracked-turn-elapsed-time.md.
  // null only when `sending` is false (nothing to show a timer for).
  turnStartedAt: string | null
  turnClockOffsetMs: number
  // Real, provider-reported token usage accumulated so far this turn —
  // 0 until the first iteration's response has actually landed. See
  // plan/ai/conversation/step-34-realtime-token-usage-budget-and-display.md.
  turnPromptTokens: number
  turnCompletionTokens: number
  turnTotalTokens: number
  // Every sub-agent the current turn has spawned so far (step 41 —
  // Sub Agents) — [] when none have been, same live-reconciled-on-
  // every-poll data source as the token fields above.
  turnSubAgents: SubAgentRun[]
  onResend: (content: string) => void
}) {
  if (messages.length === 0 && !pendingUserContent) {
    return <div className="flex flex-1 items-center justify-center text-sm text-gray-400">Send a message to start the conversation.</div>
  }

  return (
    <div className="flex-1 space-y-4 overflow-y-auto p-4">
      {messages.map((m) => (
        <Bubble key={m.createdAt + m.role} conversationId={conversationId} message={m} onResend={onResend} resendDisabled={sending} />
      ))}
      {/* Gated on `sending`, not just pendingUserContent's own
          truthiness: once the real reply is ready (sending goes false —
          see ConversationPage.tsx's own mainReplyReady comment),
          pendingUserContent still holds its last value until the watch
          entry is fully torn down (which now waits for any outliving
          sub-agents too), and the real user+assistant messages already
          render above via `messages` — showing this too would duplicate
          the just-sent message. */}
      {sending && pendingUserContent && (
        <Bubble
          conversationId={conversationId}
          message={{ role: 'user', content: pendingUserContent, createdAt: '' }}
          onResend={onResend}
          resendDisabled={sending}
        />
      )}
      {sending && turnStartedAt && (
        <div className="flex justify-start">
          <ThinkingIndicator
            startedAt={turnStartedAt}
            clockOffsetMs={turnClockOffsetMs}
            promptTokens={turnPromptTokens}
            completionTokens={turnCompletionTokens}
            totalTokens={turnTotalTokens}
          />
        </div>
      )}
      {/* Deliberately NOT gated on `sending` — a turn's own sub-agents
          can keep running well after its own reply already appeared
          above (step 41's "the reply returns early" change), and this
          is what keeps that visible live instead of only being
          reachable via a past message's own one-click historical
          lookup (Bubble's own toggleSubAgents, below). */}
      {turnSubAgents.length > 0 && (
        <div className="flex justify-start">
          <div className="max-w-lg space-y-1 rounded-md border border-gray-200 bg-white p-2 text-xs text-gray-400">
            <p className="font-medium text-gray-500">
              {turnSubAgents.some((a) => a.status === 'running') ? 'Sub-agents still working…' : 'Sub-agents'}
            </p>
            {turnSubAgents.map((agent) => (
              <SubAgentRow key={agent.id} agent={agent} />
            ))}
          </div>
        </div>
      )}
    </div>
  )
}

function Bubble({
  conversationId,
  message,
  onResend,
  resendDisabled,
}: {
  conversationId: string
  message: ConversationMessage
  onResend: (content: string) => void
  resendDisabled: boolean
}) {
  const [showError, setShowError] = useState(false)
  // Historical sub-agent detail (step 41) — fetched lazily, only on
  // the first click of "N sub-agents" below, never eagerly for every
  // message in the thread. undefined = not yet fetched, null = fetch
  // failed, [] = fetched, none found (shouldn't happen if
  // subAgentCount > 0, but handled rather than assumed).
  const [subAgents, setSubAgents] = useState<SubAgentRun[] | null | undefined>(undefined)
  const [subAgentsError, setSubAgentsError] = useState('')
  const [showSubAgents, setShowSubAgents] = useState(false)
  const isUser = message.role === 'user'

  function toggleSubAgents() {
    const next = !showSubAgents
    setShowSubAgents(next)
    if (next && subAgents === undefined && message.turnRunId) {
      fetchSubAgentsForTurn(conversationId, message.turnRunId)
        .then(setSubAgents)
        .catch((err: unknown) => {
          setSubAgentsError(err instanceof ApiError ? err.message : 'Failed to load sub-agents.')
        })
    }
  }

  return (
    <div className={`flex flex-col ${isUser ? 'items-end' : 'items-start'}`}>
      <div
        className={`max-w-lg rounded-lg px-3 py-2 text-sm ${
          message.failed ? 'border border-red-300 bg-red-50 text-red-900' : isUser ? 'bg-gray-900 text-white' : 'bg-gray-100 text-gray-900'
        }`}
      >
        <Markdown content={message.content} />
        {message.failed && (
          <div className="mt-2 flex items-center gap-2 border-t border-red-200 pt-2 text-xs">
            <span className="text-red-700">Failed to send</span>
            <button
              type="button"
              onClick={() => {
                onResend(message.content)
              }}
              disabled={resendDisabled}
              className="font-medium underline disabled:opacity-50"
            >
              Resend
            </button>
            <button
              type="button"
              onClick={() => {
                setShowError((v) => !v)
              }}
              aria-label={showError ? 'Hide error details' : 'Show error details'}
              className="ml-auto text-red-700 hover:text-red-900"
            >
              <InfoIcon />
            </button>
          </div>
        )}
        {message.failed && showError && (
          <pre className="mt-2 whitespace-pre-wrap break-words rounded bg-red-100 p-2 text-xs text-red-800">{message.error}</pre>
        )}
      </div>
      {message.createdAt && (
        <div className="mt-1 px-1 text-xs text-gray-400">
          {formatTimestamp(message.createdAt)}
          {message.durationMs != null && ` · ${formatDuration(message.durationMs)}`}
          {message.promptTokens != null &&
            ` · ↑${formatTokenCount(message.promptTokens)} ↓${formatTokenCount(message.completionTokens ?? 0)}`}
          {message.subAgentCount != null && message.turnRunId && (
            <>
              {' · '}
              <button type="button" onClick={toggleSubAgents} className="underline hover:text-gray-600">
                {message.subAgentCount} sub-agent{message.subAgentCount === 1 ? '' : 's'}
              </button>
            </>
          )}
        </div>
      )}
      {showSubAgents && (
        <div className="mt-1 max-w-lg space-y-1 rounded-md border border-gray-200 bg-white p-2">
          {subAgentsError && <p className="text-xs text-red-600">{subAgentsError}</p>}
          {subAgents === undefined && !subAgentsError && <p className="text-xs text-gray-400">Loading…</p>}
          {subAgents?.length === 0 && <p className="text-xs text-gray-400">No sub-agents found.</p>}
          {subAgents?.map((agent) => <SubAgentRow key={agent.id} agent={agent} />)}
        </div>
      )}
    </div>
  )
}

// Full datetime including seconds (plan/ai/conversation/step-14/15's
// own "should show the datetime, with seconds also") — the backend's
// RFC3339 timestamps already carry second precision; this is purely a
// display-formatting choice, not a precision one.
function formatTimestamp(iso: string): string {
  return new Date(iso).toLocaleString(undefined, {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  })
}

function InfoIcon() {
  return (
    <svg viewBox="0 0 20 20" fill="none" stroke="currentColor" className="h-4 w-4">
      <circle cx="10" cy="10" r="7.25" strokeWidth="1.3" />
      <path d="M10 9v4.5" strokeWidth="1.3" strokeLinecap="round" />
      <circle cx="10" cy="6.75" r="0.9" fill="currentColor" stroke="none" />
    </svg>
  )
}
