import { useState } from 'react'
import type { ConversationMessage } from '../../api/conversations'
import { Markdown } from '../../Shared/Components/Markdown/Markdown'

// Renders the active conversation's messages in order, user/assistant
// styled distinctly. No streaming — a sent message shows a
// "thinking…" placeholder (pendingUserContent/sending) until the
// single response resolves. A failed send (plan/ai/conversation/step-08)
// renders as its own distinct bubble with a resend action and an
// inline error-detail toggle — durable across a reload, not just this
// live session's own transient state. See
// plan/ai/conversation/step-05-frontend-chat-ui.md.
export function MessageThread({
  messages,
  pendingUserContent,
  sending,
  onResend,
}: {
  messages: ConversationMessage[]
  pendingUserContent: string | null
  sending: boolean
  onResend: (content: string) => void
}) {
  if (messages.length === 0 && !pendingUserContent) {
    return (
      <div className="flex flex-1 items-center justify-center text-sm text-gray-400">
        Send a message to start the conversation.
      </div>
    )
  }

  return (
    <div className="flex-1 space-y-4 overflow-y-auto p-4">
      {messages.map((m) => (
        <Bubble key={m.createdAt + m.role} message={m} onResend={onResend} resendDisabled={sending} />
      ))}
      {pendingUserContent && (
        <Bubble message={{ role: 'user', content: pendingUserContent, createdAt: '' }} onResend={onResend} resendDisabled={sending} />
      )}
      {sending && (
        <div className="flex justify-start">
          <div className="max-w-lg rounded-lg bg-gray-100 px-3 py-2 text-sm text-gray-400">Thinking…</div>
        </div>
      )}
    </div>
  )
}

function Bubble({
  message,
  onResend,
  resendDisabled,
}: {
  message: ConversationMessage
  onResend: (content: string) => void
  resendDisabled: boolean
}) {
  const [showError, setShowError] = useState(false)
  const isUser = message.role === 'user'

  return (
    <div className={`flex ${isUser ? 'justify-end' : 'justify-start'}`}>
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
              onClick={() => onResend(message.content)}
              disabled={resendDisabled}
              className="font-medium underline disabled:opacity-50"
            >
              Resend
            </button>
            <button
              type="button"
              onClick={() => setShowError((v) => !v)}
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
    </div>
  )
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
