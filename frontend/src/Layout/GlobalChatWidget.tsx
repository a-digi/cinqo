import { useEffect, useState } from 'react'
import { Link, useLocation } from 'react-router-dom'
import { useConversationContext } from '../config/conversation/ConversationContext'
import { fetchPlatforms, type Platform } from '../api/platforms'
import { ApiError } from '../api/client'
import type { Conversation } from '../api/conversations'
import { useConfirm } from '../Shared/Components/Modal/useConfirm'
import { IconButton } from '../Shared/Components/IconButton/IconButton'
import { TrashIcon } from '../Shared/Components/IconButton/icons'
import { LoadingSpinner } from '../Shared/Components/Loading/LoadingSpinner'
import { MessageThread } from '../Components/Conversation/MessageThread'
import { MessageComposer } from '../Components/Conversation/MessageComposer'
import { NewConversationForm } from '../Components/Conversation/NewConversationForm'

// Floating bottom-right chat widget, mounted once in Layout.tsx so it
// persists across every authenticated route (not just /conversations).
// Reads the exact same ConversationContext (step-16) ConversationPage
// itself reads — selecting or sending a message here is immediately
// reflected there too, and vice versa, since both consume one shared
// provider rather than independent copies of the same state. See
// plan/ai/conversation/step-17-global-chat-widget.md and
// step-18-global-widget-list-and-create.md (the conversation list +
// "start new" capability added on top of step 17's own narrower v1).
export function GlobalChatWidget() {
  const location = useLocation()
  const [isOpen, setIsOpen] = useState(false)
  const {
    conversations,
    selectedId,
    detail,
    sending,
    error,
    pendingUserContent,
    turnStartedAt,
    turnClockOffsetMs,
    selectConversation,
    createConversation,
    sendMessage,
    stopTurn,
    deleteConversation,
  } = useConversationContext()
  const { confirm, dialog } = useConfirm()
  const [busyId, setBusyId] = useState<string | null>(null)

  // showList/creatingNew (step 18) are local to the widget, not the
  // shared context — switching the widget's own view never affects
  // what /conversations shows if the user later navigates there.
  // Defaults to list view whenever nothing is already selected
  // (step-18's own open question 3, resolved in favor of list-first).
  const [showList, setShowList] = useState(!selectedId)
  const [creatingNew, setCreatingNew] = useState(false)
  const [platforms, setPlatforms] = useState<Platform[]>([])
  const [platformsError, setPlatformsError] = useState<string | null>(null)

  useEffect(() => {
    fetchPlatforms()
      .then(setPlatforms)
      .catch((err) => setPlatformsError(err instanceof ApiError ? err.message : 'Failed to load platforms.'))
  }, [])

  // Hidden entirely on /conversations itself — that page already shows
  // this same conversation full-size; a second copy of it floating on
  // top would be redundant, not useful.
  if (location.pathname.startsWith('/conversations')) {
    return null
  }

  function openList() {
    setCreatingNew(false)
    setShowList(true)
  }

  function openNew() {
    setShowList(false)
    setCreatingNew(true)
  }

  function handleSelect(id: string) {
    setCreatingNew(false)
    setShowList(false)
    selectConversation(id)
  }

  async function handleCreate(input: { platformId: string; model?: string }) {
    try {
      await createConversation(input)
      setCreatingNew(false)
    } catch {
      // createConversation already surfaces its own failure via the
      // shared context's own `error` state — nothing extra to do here.
    }
  }

  async function handleDelete(c: Conversation) {
    const ok = await confirm({
      title: 'Delete conversation',
      message: `Delete "${c.title}"? This cannot be undone.`,
      danger: true,
    })
    if (!ok) return
    setBusyId(c.id)
    try {
      // deleteConversation itself clears selectedId/detail when the
      // deleted conversation was the one open — the existing render
      // condition (showList || !selectedId) then falls back to the
      // list view with no extra handling needed here.
      await deleteConversation(c.id)
    } finally {
      setBusyId(null)
    }
  }

  if (!isOpen) {
    return (
      <>
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
        {dialog}
      </>
    )
  }

  const selectedTitle = conversations?.find((c) => c.id === selectedId)?.title
  const headerTitle = creatingNew ? 'New conversation' : showList || !selectedId ? 'Conversations' : (selectedTitle ?? 'Conversation')

  return (
    <>
      <div className="pointer-events-none fixed bottom-4 right-4 z-50">
        <div className="pointer-events-auto flex max-h-[70vh] w-[380px] max-w-[calc(100vw-2rem)] flex-col rounded-lg border border-gray-200 bg-white shadow-xl">
        <div className="flex items-center gap-1 border-b border-gray-200 px-2 py-2">
          <button
            type="button"
            onClick={openList}
            aria-label="Show conversations"
            title="Show conversations"
            className="shrink-0 rounded p-1.5 text-gray-500 hover:bg-gray-100"
          >
            <ListIcon />
          </button>
          <button
            type="button"
            onClick={openNew}
            aria-label="Start a new conversation"
            title="Start a new conversation"
            className="shrink-0 rounded p-1.5 text-gray-500 hover:bg-gray-100"
          >
            <PlusIcon />
          </button>
          <div className="min-w-0 flex-1 truncate px-1 text-sm font-medium text-gray-900">{headerTitle}</div>
          <Link
            to="/conversations"
            aria-label="Open full conversation view"
            title="Open full conversation view"
            className="shrink-0 rounded p-1.5 text-gray-500 hover:bg-gray-100"
            onClick={() => setIsOpen(false)}
          >
            <ExpandIcon />
          </Link>
          <button
            type="button"
            onClick={() => setIsOpen(false)}
            aria-label="Close chat"
            title="Close chat"
            className="shrink-0 rounded p-1.5 text-gray-500 hover:bg-gray-100"
          >
            <CloseIcon />
          </button>
        </div>

        {(error || platformsError) && (
          <p className="border-b border-red-100 bg-red-50 px-3 py-1.5 text-xs text-red-600">{error ?? platformsError}</p>
        )}

        {creatingNew ? (
          platforms.length === 0 ? (
            <div className="flex flex-1 items-center justify-center p-4 text-center text-sm text-gray-400">
              {platformsError ? 'Could not load AI platforms.' : 'No AI platform configured yet.'}
            </div>
          ) : (
            <NewConversationForm platforms={platforms} onCreate={handleCreate} onCancel={openList} />
          )
        ) : showList || !selectedId ? (
          <ConversationList
            conversations={conversations}
            selectedId={selectedId}
            busyId={busyId}
            onSelect={handleSelect}
            onDelete={handleDelete}
          />
        ) : !detail ? (
          <div className="flex flex-1 items-center justify-center p-6">
            <LoadingSpinner label="Loading conversation…" />
          </div>
        ) : (
          <>
            <MessageThread
              messages={detail.messages}
              pendingUserContent={pendingUserContent}
              sending={sending}
              turnStartedAt={turnStartedAt}
              turnClockOffsetMs={turnClockOffsetMs}
              onResend={sendMessage}
            />
            <MessageComposer platformLabel={`${detail.platformId} · ${detail.model}`} onSend={sendMessage} onStop={stopTurn} disabled={sending} />
          </>
        )}
        </div>
      </div>
      {dialog}
    </>
  )
}

function ConversationList({
  conversations,
  selectedId,
  busyId,
  onSelect,
  onDelete,
}: {
  conversations: Conversation[] | null
  selectedId: string | null
  busyId: string | null
  onSelect: (id: string) => void
  onDelete: (c: Conversation) => void
}) {
  if (!conversations || conversations.length === 0) {
    return (
      <div className="flex flex-1 items-center justify-center p-4 text-center text-sm text-gray-400">
        No conversations yet — use the + button above to start one.
      </div>
    )
  }

  return (
    <div className="flex-1 overflow-y-auto">
      {conversations.map((c) => (
        <div
          key={c.id}
          className={`flex items-center border-b border-gray-100 hover:bg-gray-50 ${c.id === selectedId ? 'bg-gray-50' : ''}`}
        >
          <button
            type="button"
            onClick={() => onSelect(c.id)}
            className={`min-w-0 flex-1 px-3 py-2 text-left text-sm ${c.id === selectedId ? 'font-medium text-gray-900' : 'text-gray-700'}`}
          >
            <div className="truncate">{c.title}</div>
            <div className="text-xs text-gray-400">{new Date(c.startedAt).toLocaleString()}</div>
          </button>
          <div className="mr-1 shrink-0">
            <IconButton icon={<TrashIcon />} label="Delete" onClick={() => onDelete(c)} disabled={busyId === c.id} variant="danger" />
          </div>
        </div>
      ))}
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

function ListIcon() {
  return (
    <svg viewBox="0 0 20 20" fill="none" stroke="currentColor" className="h-4 w-4">
      <path d="M4 5.5h12M4 10h12M4 14.5h12" strokeWidth="1.4" strokeLinecap="round" />
    </svg>
  )
}

function PlusIcon() {
  return (
    <svg viewBox="0 0 20 20" fill="none" stroke="currentColor" className="h-4 w-4">
      <path d="M10 5v10M5 10h10" strokeWidth="1.6" strokeLinecap="round" />
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
