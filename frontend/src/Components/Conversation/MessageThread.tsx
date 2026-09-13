import type { ConversationMessage } from '../../api/conversations'

// Renders the active conversation's messages in order, user/assistant
// styled distinctly. No streaming — a sent message shows a
// "thinking…" placeholder (pendingUserContent/sending) until the
// single response resolves. See
// plan/ai/conversation/step-05-frontend-chat-ui.md.
export function MessageThread({
  messages,
  pendingUserContent,
  sending,
}: {
  messages: ConversationMessage[]
  pendingUserContent: string | null
  sending: boolean
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
        <Bubble key={m.createdAt + m.role} role={m.role} content={m.content} />
      ))}
      {pendingUserContent && <Bubble role="user" content={pendingUserContent} />}
      {sending && (
        <div className="flex justify-start">
          <div className="max-w-lg rounded-lg bg-gray-100 px-3 py-2 text-sm text-gray-400">Thinking…</div>
        </div>
      )}
    </div>
  )
}

function Bubble({ role, content }: { role: 'user' | 'assistant'; content: string }) {
  const isUser = role === 'user'
  return (
    <div className={`flex ${isUser ? 'justify-end' : 'justify-start'}`}>
      <div
        className={`max-w-lg whitespace-pre-wrap rounded-lg px-3 py-2 text-sm ${
          isUser ? 'bg-gray-900 text-white' : 'bg-gray-100 text-gray-900'
        }`}
      >
        {content}
      </div>
    </div>
  )
}
