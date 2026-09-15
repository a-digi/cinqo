import { useEffect, useState } from 'react'
import type { ConversationDetail } from '../../api/conversations'
import { fetchPlatforms, type Platform } from '../../api/platforms'
import { ApiError } from '../../api/client'
import { useConversationContext } from '../../config/conversation/ConversationContext'
import { LoadingSpinner } from '../../Shared/Components/Loading/LoadingSpinner'
import { ConversationSidebar } from './ConversationSidebar'
import { MessageThread } from './MessageThread'
import { MessageComposer } from './MessageComposer'
import { NewConversationForm } from './NewConversationForm'

// The whole screen: sidebar + thread + composer — a real top-level
// destination, not a floating per-page widget (a deliberate divergence
// from the reference — see this step's own design doc).
//
// A conversation's platform+model are chosen once, at creation, and
// fixed for its whole lifetime — see
// plan/ai/conversation/step-07-fixed-platform-and-model-per-conversation.md.
// "New conversation" no longer creates instantly; it shows an inline
// picker (creatingNew) in the right-hand pane first.
//
// Conversations/selectedId/detail/sending/pendingUserContent/error all
// come from the shared ConversationContext (plan/ai/conversation/
// step-16) now, not local state — the same provider step-17's own
// floating widget reads, so both show the exact same conversation.
// platforms/busyId/isSidebarCollapsed/creatingNew stay local — they're
// pure page-UI concerns the widget has no equivalent of.
export function ConversationPage() {
  const {
    conversations,
    selectedId,
    detail,
    loading,
    error,
    turnWatches,
    selectConversation,
    createConversation,
    sendMessage,
    stopTurn,
    renameConversation,
    deleteConversation,
  } = useConversationContext()
  // Derived from the shared per-conversation-ID registry
  // (plan/ai/conversation/step-29-frontend-per-conversation-turn-watch-registry.md)
  // for whichever conversation is currently selected — the page itself
  // still only ever shows one conversation's own thread at a time,
  // even though the registry can track several concurrently.
  const watch = selectedId ? turnWatches[selectedId] : undefined
  const sending = !!watch
  const pendingUserContent = watch?.pendingUserContent ?? null
  const turnStartedAt = watch?.turnStartedAt ?? null
  const turnClockOffsetMs = watch?.turnClockOffsetMs ?? 0
  const [platforms, setPlatforms] = useState<Platform[] | null>(null)
  const [platformsError, setPlatformsError] = useState<string | null>(null)
  const [busyId, setBusyId] = useState<string | null>(null)
  const [isSidebarCollapsed, setIsSidebarCollapsed] = useState(false)
  const [creatingNew, setCreatingNew] = useState(false)

  useEffect(() => {
    fetchPlatforms()
      .then(setPlatforms)
      .catch((err) => setPlatformsError(err instanceof ApiError ? err.message : 'Failed to load platforms.'))
  }, [])

  const handleSelect = (id: string) => {
    setCreatingNew(false)
    selectConversation(id)
  }

  const handleStartCreate = () => {
    setCreatingNew(true)
  }

  const handleCreate = async (input: { platformId: string; model?: string }) => {
    try {
      await createConversation(input)
      setCreatingNew(false)
    } catch {
      // createConversation already surfaces its own failure via the
      // shared context's own `error` state — nothing extra to do here.
    }
  }

  const handleRename = async (id: string, title: string) => {
    setBusyId(id)
    try {
      await renameConversation(id, title)
    } finally {
      setBusyId(null)
    }
  }

  const handleDelete = async (id: string) => {
    setBusyId(id)
    try {
      await deleteConversation(id)
    } finally {
      setBusyId(null)
    }
  }

  const combinedError = error ?? platformsError

  if (combinedError && !conversations) {
    return <p className="p-6 text-sm text-red-600">{combinedError}</p>
  }

  if (!platforms || loading || !conversations) {
    return (
      <div className="flex min-h-64 items-center justify-center">
        <LoadingSpinner label="Loading conversations…" />
      </div>
    )
  }

  return (
    <div className="flex h-[calc(100vh-4rem)]">
      {!isSidebarCollapsed && (
        <ConversationSidebar
          conversations={conversations}
          selectedId={selectedId}
          busyId={busyId}
          onSelect={handleSelect}
          onCreate={handleStartCreate}
          onRename={handleRename}
          onDelete={handleDelete}
        />
      )}

      <div className="flex flex-1 flex-col">
        <div className="flex items-center border-b border-gray-200 px-3 py-2">
          <button
            type="button"
            onClick={() => setIsSidebarCollapsed((v) => !v)}
            aria-label={isSidebarCollapsed ? 'Show conversation list' : 'Hide conversation list'}
            className="rounded p-1.5 text-gray-500 hover:bg-gray-100"
          >
            <PanelIcon open={!isSidebarCollapsed} />
          </button>
        </div>

        {combinedError && <p className="border-b border-red-100 bg-red-50 px-4 py-2 text-sm text-red-600">{combinedError}</p>}

        {creatingNew && <NewConversationForm platforms={platforms} onCreate={handleCreate} onCancel={() => setCreatingNew(false)} />}

        {!creatingNew && !selectedId && (
          <div className="flex flex-1 items-center justify-center text-sm text-gray-400">
            Select or create a conversation.
          </div>
        )}

        {!creatingNew && selectedId && !detail && (
          <div className="flex flex-1 items-center justify-center">
            <LoadingSpinner label="Loading conversation…" />
          </div>
        )}

        {!creatingNew && selectedId && detail && (
          <>
            <MessageThread
              messages={detail.messages}
              pendingUserContent={pendingUserContent}
              sending={sending}
              turnStartedAt={turnStartedAt}
              turnClockOffsetMs={turnClockOffsetMs}
              onResend={(content) => void sendMessage(selectedId, content)}
            />
            <MessageComposer
              platformLabel={platformLabel(platforms, detail)}
              onSend={(content) => void sendMessage(selectedId, content)}
              onStop={() => void stopTurn(selectedId)}
              disabled={sending}
            />
          </>
        )}
      </div>
    </div>
  )
}

function platformLabel(platforms: Platform[], detail: ConversationDetail): string {
  const name = platforms.find((p) => p.id === detail.platformId)?.name ?? detail.platformId
  return `${name} · ${detail.model}`
}

// A panel outline with its left "sidebar" strip filled when the
// conversation list is showing, hollow when it's collapsed.
function PanelIcon({ open }: { open: boolean }) {
  return (
    <svg viewBox="0 0 20 20" fill="none" stroke="currentColor" className="h-5 w-5">
      <rect x="3" y="4" width="14" height="12" rx="1.5" strokeWidth="1.2" />
      <rect x="3" y="4" width="3.5" height="12" rx="1" fill={open ? 'currentColor' : 'none'} strokeWidth="1.2" />
    </svg>
  )
}
