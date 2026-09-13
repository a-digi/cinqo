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
export function ConversationPage() {
  const [platforms, setPlatforms] = useState<Platform[] | null>(null)
  const [conversations, setConversations] = useState<Conversation[] | null>(null)
  const [selectedId, setSelectedId] = useState<string | null>(null)
  const [detail, setDetail] = useState<ConversationDetail | null>(null)
  const [selectedPlatform, setSelectedPlatform] = useState('')
  const [busyId, setBusyId] = useState<string | null>(null)
  const [sending, setSending] = useState(false)
  const [pendingUserContent, setPendingUserContent] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    Promise.all([fetchPlatforms(), fetchConversations()])
      .then(([p, c]) => {
        setPlatforms(p)
        if (p.length > 0) setSelectedPlatform(p[0].id)
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
    setSelectedId(id)
    setDetail(null)
    loadDetail(id)
  }

  const handleCreate = async () => {
    try {
      const created = await createConversation()
      setConversations((prev) => (prev ? [created, ...prev] : [created]))
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
    if (!selectedId || !selectedPlatform) return
    setError(null)
    setPendingUserContent(content)
    setSending(true)
    try {
      await sendMessage(selectedId, { platformId: selectedPlatform, content })
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
      <ConversationSidebar
        conversations={conversations}
        selectedId={selectedId}
        busyId={busyId}
        onSelect={handleSelect}
        onCreate={handleCreate}
        onRename={handleRename}
        onDelete={handleDelete}
      />

      <div className="flex flex-1 flex-col">
        {error && <p className="border-b border-red-100 bg-red-50 px-4 py-2 text-sm text-red-600">{error}</p>}

        {!selectedId && (
          <div className="flex flex-1 items-center justify-center text-sm text-gray-400">
            Select or create a conversation.
          </div>
        )}

        {selectedId && !detail && (
          <div className="flex flex-1 items-center justify-center">
            <LoadingSpinner label="Loading conversation…" />
          </div>
        )}

        {selectedId && detail && (
          <>
            <MessageThread messages={detail.messages} pendingUserContent={pendingUserContent} sending={sending} />
            <MessageComposer
              platforms={platforms}
              platform={selectedPlatform}
              onPlatformChange={setSelectedPlatform}
              onSend={handleSend}
              disabled={sending}
            />
          </>
        )}
      </div>
    </div>
  )
}
