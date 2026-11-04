import { useEffect, useState } from 'react'
import { Navigate, useNavigate, useParams } from 'react-router-dom'
import type { ConversationDetail, OrchestrationMode } from '../../api/conversations'
import { fetchPlatforms, type Platform } from '../../api/platforms'
import { fetchTools, type Tool } from '../../api/tools'
import { ApiError } from '../../api/client'
import { useConversationContext } from '../../config/conversation/ConversationContext'
import { LoadingSpinner } from '../../Shared/Components/Loading/LoadingSpinner'
import { ConversationSidebar } from './ConversationSidebar'
import { MessageThread } from './MessageThread'
import { MessageComposer } from './MessageComposer'
import { NewConversationForm } from './NewConversationForm'
import { CONVERSATIONS_PATH, conversationPath, isConversationId } from './conversationPaths'

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
    clearSelection,
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
  // !watch.mainReplyReady (step 41 follow-up): once the orchestrator's
  // own reply is fetched, the watch entry can stay alive (still
  // tracking any sub-agents that outlive it), but ThinkingIndicator
  // should stop showing — the real reply already exists as its own
  // message by then.
  const sending = !!watch && !watch.mainReplyReady
  const pendingUserContent = watch?.pendingUserContent ?? null
  const turnStartedAt = watch?.turnStartedAt ?? null
  const turnClockOffsetMs = watch?.turnClockOffsetMs ?? 0
  const turnPromptTokens = watch?.promptTokens ?? 0
  const turnCompletionTokens = watch?.completionTokens ?? 0
  const turnTotalTokens = watch?.totalTokens ?? 0
  const turnSubAgents = watch?.subAgents ?? []
  const turnPlan = watch?.plan ?? []
  const [platforms, setPlatforms] = useState<Platform[] | null>(null)
  const [platformsError, setPlatformsError] = useState<string | null>(null)
  const [busyId, setBusyId] = useState<string | null>(null)
  const [isSidebarCollapsed, setIsSidebarCollapsed] = useState(false)
  const [creatingNew, setCreatingNew] = useState(false)
  // tools (step 41) — every installed tool, for the sidebar's own
  // "filter by tool" dropdown. A fetch failure here just leaves the
  // dropdown showing no tool options; it never blocks the page the way
  // a platforms failure does (platforms are required to create a new
  // conversation at all, tools are only used for an optional filter).
  const [tools, setTools] = useState<Tool[]>([])

  // The address names the conversation shown (`/conversations/<uuid>`): it is the source of truth, so back, forward, reload and a pasted link
  // all work. `routeId` is what the address holds, `urlId` only when it is a real id; text that is not an id never reaches the server.
  const { id: routeId } = useParams()
  const navigate = useNavigate()
  const urlId = isConversationId(routeId) ? routeId : null

  // The address names a conversation other than the selected one (a link opened, the back button, a click in the list): select it.
  useEffect(() => {
    if (urlId && urlId !== selectedId) {
      setCreatingNew(false)
      selectConversation(urlId)
    }
  }, [urlId, selectedId, selectConversation])

  // A bare /conversations with a conversation already selected (opened from the floating chat, or one just created, or the picker closed
  // again) shows that conversation, so the address always names what is shown. Not while the picker is open, while a delete is under way
  // (the delete goes to the list itself; if it fails this puts the address back), or when the selected conversation could not be opened.
  useEffect(() => {
    if (!routeId && selectedId && !creatingNew && busyId === null && !(error && !detail)) {
      void navigate(conversationPath(selectedId), { replace: true })
    }
  }, [routeId, selectedId, creatingNew, busyId, error, detail, navigate])

  useEffect(() => {
    fetchPlatforms()
      .then(setPlatforms)
      .catch((err: unknown) => {
        setPlatformsError(err instanceof ApiError ? err.message : 'Failed to load platforms.')
      })
    fetchTools()
      .then(setTools)
      .catch(() => {
        // Best-effort — see this state's own doc comment above.
      })
  }, [])

  const handleSelect = (id: string) => {
    setCreatingNew(false)
    void navigate(conversationPath(id))
  }

  const handleStartCreate = () => {
    setCreatingNew(true)
    if (routeId) void navigate(CONVERSATIONS_PATH)
  }

  const handleCreate = async (input: { platformId: string; model?: string; orchestrationMode: OrchestrationMode }) => {
    try {
      const created = await createConversation(input)
      setCreatingNew(false)
      void navigate(conversationPath(created.id))
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
    // Deleting the open conversation goes to the list first, so the address never names a conversation that is gone.
    if (id === selectedId) void navigate(CONVERSATIONS_PATH, { replace: true })
    try {
      await deleteConversation(id)
    } finally {
      setBusyId(null)
    }
  }

  const backToList = () => {
    clearSelection()
    void navigate(CONVERSATIONS_PATH)
  }

  // An address that is not a conversation id (text after /conversations/ that is not a UUID) is the list.
  if (routeId !== undefined && urlId === null) {
    return <Navigate to={CONVERSATIONS_PATH} replace />
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
    <div className="cinqo-stagger cinqo-v-fade flex h-[calc(100vh-4rem)]">
      {!isSidebarCollapsed && (
        <ConversationSidebar
          conversations={conversations}
          tools={tools}
          selectedId={selectedId}
          busyId={busyId}
          onSelect={handleSelect}
          onCreate={handleStartCreate}
          onRename={(id, title) => {
            void handleRename(id, title)
          }}
          onDelete={(id) => {
            void handleDelete(id)
          }}
        />
      )}

      <div className="flex flex-1 flex-col">
        <div className="flex items-center border-b border-gray-200 px-3 py-2">
          <button
            type="button"
            onClick={() => {
              setIsSidebarCollapsed((v) => !v)
            }}
            aria-label={isSidebarCollapsed ? 'Show conversation list' : 'Hide conversation list'}
            className="rounded p-1.5 text-gray-500 hover:bg-gray-100"
          >
            <PanelIcon open={!isSidebarCollapsed} />
          </button>
        </div>

        {combinedError && <p className="border-b border-red-100 bg-red-50 px-4 py-2 text-sm text-red-600">{combinedError}</p>}

        {creatingNew && (
          <NewConversationForm
            platforms={platforms}
            onCreate={(input) => {
              void handleCreate(input)
            }}
            onCancel={() => {
              setCreatingNew(false)
            }}
          />
        )}

        {!creatingNew && !selectedId && (
          <div className="flex flex-1 items-center justify-center text-sm text-gray-400">Select or create a conversation.</div>
        )}

        {!creatingNew && selectedId && !detail && error && (
          <div role="alert" className="flex flex-1 flex-col items-center justify-center gap-3 px-6 text-center text-sm text-gray-600">
            <p className="font-medium text-gray-900">This conversation could not be opened.</p>
            <p>It may have been deleted, or it may belong to someone else.</p>
            <button
              type="button"
              onClick={backToList}
              className="rounded-md border border-gray-300 bg-white px-3 py-1.5 font-medium text-gray-700 hover:bg-gray-50"
            >
              Back to the conversations
            </button>
          </div>
        )}

        {!creatingNew && selectedId && !detail && !error && (
          <div className="flex flex-1 items-center justify-center">
            <LoadingSpinner label="Loading conversation…" />
          </div>
        )}

        {!creatingNew && selectedId && detail && (
          <>
            <MessageThread
              conversationId={selectedId}
              messages={detail.messages}
              pendingUserContent={pendingUserContent}
              sending={sending}
              turnStartedAt={turnStartedAt}
              turnClockOffsetMs={turnClockOffsetMs}
              turnPromptTokens={turnPromptTokens}
              turnCompletionTokens={turnCompletionTokens}
              turnTotalTokens={turnTotalTokens}
              turnUsageExtras={watch?.usageExtras}
              turnSubAgents={turnSubAgents}
              turnPlan={turnPlan}
              orchestrationMode={detail.orchestrationMode}
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
  return `${name} · ${detail.model}${detail.orchestrationMode === 'composer' ? ' · composer' : ''}`
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
