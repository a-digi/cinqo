import { useEffect, useState } from 'react'
import {
  fetchConversations,
  createConversation,
  fetchConversation,
  renameConversation,
  deleteConversation,
  sendMessage,
  type Conversation,
  type ConversationDetail,
} from '../../api/conversations'
import { fetchPlatforms, type Platform } from '../../api/platforms'
import { ApiError } from '../../api/client'
import { LoadingSpinner } from '../../Shared/Components/Loading/LoadingSpinner'
import { ConversationSidebar } from './ConversationSidebar'
import { MessageThread } from './MessageThread'
import { MessageComposer } from './MessageComposer'

// The whole screen: sidebar + thread + composer — a real top-level
// destination, not a floating per-page widget (a deliberate divergence
// from the reference — see this step's own design doc).
//
// A conversation's platform+model are chosen once, at creation, and
// fixed for its whole lifetime — see
// plan/ai/conversation/step-07-fixed-platform-and-model-per-conversation.md.
// "New conversation" no longer creates instantly; it shows an inline
// picker (creatingNew) in the right-hand pane first.
export function ConversationPage() {
  const [platforms, setPlatforms] = useState<Platform[] | null>(null)
  const [conversations, setConversations] = useState<Conversation[] | null>(null)
  const [selectedId, setSelectedId] = useState<string | null>(null)
  const [detail, setDetail] = useState<ConversationDetail | null>(null)
  const [busyId, setBusyId] = useState<string | null>(null)
  const [sending, setSending] = useState(false)
  const [pendingUserContent, setPendingUserContent] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [isSidebarCollapsed, setIsSidebarCollapsed] = useState(false)
  const [creatingNew, setCreatingNew] = useState(false)

  useEffect(() => {
    Promise.all([fetchPlatforms(), fetchConversations()])
      .then(([p, c]) => {
        setPlatforms(p)
        setConversations(c)
      })
      .catch((err) => setError(err instanceof ApiError ? err.message : 'Failed to load conversations.'))
  }, [])

  const loadDetail = (id: string) => {
    fetchConversation(id)
      .then((d) => setDetail(d))
      .catch((err) => setError(err instanceof ApiError ? err.message : 'Failed to load conversation.'))
  }

  const handleSelect = (id: string) => {
    setCreatingNew(false)
    setSelectedId(id)
    setDetail(null)
    loadDetail(id)
  }

  const handleStartCreate = () => {
    setError(null)
    setSelectedId(null)
    setDetail(null)
    setCreatingNew(true)
  }

  const handleCreate = async (input: { platformId: string; model?: string }) => {
    try {
      const created = await createConversation(input)
      setConversations((prev) => (prev ? [created, ...prev] : [created]))
      setCreatingNew(false)
      setSelectedId(created.id)
      setDetail({ ...created, messages: [] })
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to create conversation.')
    }
  }

  const handleRename = async (id: string, title: string) => {
    setBusyId(id)
    try {
      const updated = await renameConversation(id, title)
      setConversations((prev) => (prev ? prev.map((c) => (c.id === id ? updated : c)) : prev))
      setDetail((prev) => (prev && prev.id === id ? { ...prev, title: updated.title } : prev))
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to rename conversation.')
    } finally {
      setBusyId(null)
    }
  }

  const handleDelete = async (id: string) => {
    setBusyId(id)
    try {
      await deleteConversation(id)
      setConversations((prev) => (prev ? prev.filter((c) => c.id !== id) : prev))
      if (selectedId === id) {
        setSelectedId(null)
        setDetail(null)
      }
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to delete conversation.')
    } finally {
      setBusyId(null)
    }
  }

  const handleSend = async (content: string) => {
    if (!selectedId) return
    setError(null)
    setPendingUserContent(content)
    setSending(true)
    try {
      await sendMessage(selectedId, { content })
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to send message.')
    } finally {
      // Refetch either way — a provider failure still records the
      // user's own message server-side (plan/ai/conversation/step-02),
      // so the real state is always worth showing rather than
      // reconstructing it optimistically.
      try {
        if (selectedId) await loadDetailAsync(selectedId)
      } finally {
        setPendingUserContent(null)
        setSending(false)
      }
    }
  }

  const loadDetailAsync = async (id: string) => {
    try {
      const d = await fetchConversation(id)
      setDetail(d)
    } catch {
      // Best-effort refresh — a failure here doesn't need its own
      // error message on top of whatever handleSend already surfaced.
    }
  }

  if (error && !conversations) {
    return <p className="p-6 text-sm text-red-600">{error}</p>
  }

  if (!platforms || !conversations) {
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

        {error && <p className="border-b border-red-100 bg-red-50 px-4 py-2 text-sm text-red-600">{error}</p>}

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
            <MessageThread messages={detail.messages} pendingUserContent={pendingUserContent} sending={sending} />
            <MessageComposer platformLabel={platformLabel(platforms, detail)} onSend={handleSend} disabled={sending} />
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

// The one-time platform (and, for OpenAI/Anthropic, model) picker
// shown before a conversation exists — not a modal, reuses the same
// pane the thread/composer render into once a conversation is picked.
function NewConversationForm({
  platforms,
  onCreate,
  onCancel,
}: {
  platforms: Platform[]
  onCreate: (input: { platformId: string; model?: string }) => void
  onCancel: () => void
}) {
  const [platformId, setPlatformId] = useState(platforms[0]?.id ?? '')
  const [model, setModel] = useState(platforms[0]?.models[0] ?? '')

  const selectedPlatform = platforms.find((p) => p.id === platformId)
  const needsModel = (selectedPlatform?.models.length ?? 0) > 0

  const handlePlatformChange = (id: string) => {
    setPlatformId(id)
    const next = platforms.find((p) => p.id === id)
    setModel(next?.models[0] ?? '')
  }

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    onCreate({ platformId, model: needsModel ? model : undefined })
  }

  return (
    <div className="flex flex-1 items-center justify-center">
      <form onSubmit={handleSubmit} className="w-full max-w-sm space-y-3 rounded-lg border border-gray-200 p-5">
        <h2 className="text-sm font-semibold text-gray-900">New conversation</h2>
        <div>
          <label className="mb-1 block text-xs font-medium text-gray-500">Platform</label>
          <select
            value={platformId}
            onChange={(e) => handlePlatformChange(e.target.value)}
            required
            className="w-full rounded border border-gray-300 px-2 py-1.5 text-sm"
          >
            {platforms.map((p) => (
              <option key={p.id} value={p.id}>
                {p.name}
              </option>
            ))}
          </select>
        </div>
        {needsModel && (
          <div>
            <label className="mb-1 block text-xs font-medium text-gray-500">Model</label>
            <select
              value={model}
              onChange={(e) => setModel(e.target.value)}
              required
              className="w-full rounded border border-gray-300 px-2 py-1.5 text-sm"
            >
              {selectedPlatform?.models.map((m) => (
                <option key={m} value={m}>
                  {m}
                </option>
              ))}
            </select>
          </div>
        )}
        <p className="text-xs text-gray-400">The platform and model can&apos;t be changed after the conversation is created.</p>
        <div className="flex justify-end gap-2">
          <button type="button" onClick={onCancel} className="rounded border border-gray-300 px-3 py-1.5 text-sm text-gray-700 hover:bg-gray-50">
            Cancel
          </button>
          <button type="submit" className="rounded bg-gray-900 px-3 py-1.5 text-sm font-medium text-white hover:bg-gray-800">
            Create
          </button>
        </div>
      </form>
    </div>
  )
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
