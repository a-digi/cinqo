import { useState } from 'react'
import { Link, useLocation } from 'react-router-dom'
import { useConversationContext } from '../config/conversation/ConversationContext'
import { Dropdown } from '../Shared/Components/Dropdown/Dropdown'
import { LoadingSpinner } from '../Shared/Components/Loading/LoadingSpinner'
import { MessageThread } from '../Components/Conversation/MessageThread'
import { MessageComposer } from '../Components/Conversation/MessageComposer'

// Floating bottom-right chat widget, mounted once in Layout.tsx so it
// persists across every authenticated route (not just /conversations).
// Reads the exact same ConversationContext (step-16) ConversationPage
// itself reads — selecting or sending a message here is immediately
// reflected there too, and vice versa, since both consume one shared
// provider rather than independent copies of the same state. See
// plan/ai/conversation/step-17-global-chat-widget.md.
//
// Deliberately can only SELECT an existing conversation, never create
// one — starting a brand-new conversation needs a platform/model
// choice this compact panel has no picker for; that stays on the full
// /conversations page (linked via "Open full view" below).
export function GlobalChatWidget() {
  const location = useLocation()
  const [isOpen, setIsOpen] = useState(false)
  const { conversations, selectedId, detail, sending, error, pendingUserContent, selectConversation, sendMessage } =
    useConversationContext()

  // Hidden entirely on /conversations itself — that page already shows
  // this same conversation full-size; a second copy of it floating on
  // top would be redundant, not useful.
  if (location.pathname.startsWith('/conversations')) {
    return null
  }

  if (!isOpen) {
    return (
      <div className="pointer-events-none fixed bottom-4 right-4 z-50">
        <button
          type="button"
          onClick={() => setIsOpen(true)}
          aria-label="Open chat"
          className="pointer-events-auto flex h-12 w-12 items-center justify-center rounded-full bg-gray-900 text-white shadow-lg hover:bg-gray-800"
        >
          <ChatIcon />
        </button>
      </div>
    )
  }

  return (
    <div className="pointer-events-none fixed bottom-4 right-4 z-50">
      <div className="pointer-events-auto flex max-h-[70vh] w-[380px] max-w-[calc(100vw-2rem)] flex-col rounded-lg border border-gray-200 bg-white shadow-xl">
        <div className="flex items-center gap-2 border-b border-gray-200 px-3 py-2">
          <div className="min-w-0 flex-1">
            {conversations && conversations.length > 0 ? (
              <Dropdown
                options={conversations.map((c) => ({ value: c.id, label: c.title }))}
                value={selectedId ?? ''}
                onChange={selectConversation}
                placeholder="Select a conversation"
              />
            ) : (
              <span className="text-sm text-gray-400">No conversations yet</span>
            )}
          </div>
          <Link
            to="/conversations"
            aria-label="Open full conversation view"
            className="shrink-0 rounded p-1.5 text-gray-500 hover:bg-gray-100"
            onClick={() => setIsOpen(false)}
          >
            <ExpandIcon />
          </Link>
          <button
            type="button"
            onClick={() => setIsOpen(false)}
            aria-label="Close chat"
            className="shrink-0 rounded p-1.5 text-gray-500 hover:bg-gray-100"
          >
            <CloseIcon />
          </button>
        </div>

        {error && <p className="border-b border-red-100 bg-red-50 px-3 py-1.5 text-xs text-red-600">{error}</p>}

        {!conversations || conversations.length === 0 ? (
          <div className="flex flex-1 items-center justify-center p-4 text-center text-sm text-gray-400">
            <span>
              No conversations yet.{' '}
              <Link to="/conversations" className="underline" onClick={() => setIsOpen(false)}>
                Start one
              </Link>
              .
            </span>
          </div>
        ) : !selectedId ? (
          <div className="flex flex-1 items-center justify-center p-4 text-center text-sm text-gray-400">
            Select a conversation above to continue chatting.
          </div>
        ) : !detail ? (
          <div className="flex flex-1 items-center justify-center p-6">
            <LoadingSpinner label="Loading conversation…" />
          </div>
        ) : (
          <>
            <MessageThread messages={detail.messages} pendingUserContent={pendingUserContent} sending={sending} onResend={sendMessage} />
            <MessageComposer platformLabel={`${detail.platformId} · ${detail.model}`} onSend={sendMessage} disabled={sending} />
          </>
        )}
      </div>
    </div>
  )
}

function ChatIcon() {
  return (
    <svg viewBox="0 0 20 20" fill="none" stroke="currentColor" className="h-5 w-5">
      <path
        d="M3 5.5A1.5 1.5 0 0 1 4.5 4h11A1.5 1.5 0 0 1 17 5.5v6a1.5 1.5 0 0 1-1.5 1.5H8l-3.5 3v-3H4.5A1.5 1.5 0 0 1 3 11.5v-6z"
        strokeWidth="1.3"
        strokeLinejoin="round"
      />
    </svg>
  )
}

function ExpandIcon() {
  return (
    <svg viewBox="0 0 20 20" fill="none" stroke="currentColor" className="h-4 w-4">
      <path d="M8 4H4v4M12 16h4v-4M4 4l5 5M16 16l-5-5" strokeWidth="1.4" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  )
}

function CloseIcon() {
  return (
    <svg viewBox="0 0 20 20" fill="none" stroke="currentColor" className="h-4 w-4">
      <path d="M5.5 5.5l9 9M14.5 5.5l-9 9" strokeWidth="1.6" strokeLinecap="round" />
    </svg>
  )
}
