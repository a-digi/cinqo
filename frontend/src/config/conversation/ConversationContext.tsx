import { createContext, useCallback, useContext, useEffect, useRef, useState, type ReactNode } from 'react'
import { useAuth } from '../../Components/Auth/AuthContext'
import {
  fetchConversations,
  fetchConversation,
  createConversation as createConversationApi,
  sendMessage as sendMessageApi,
  renameConversation as renameConversationApi,
  deleteConversation as deleteConversationApi,
  type Conversation,
  type ConversationDetail,
} from '../../api/conversations'
import { ApiError } from '../../api/client'

export interface ConversationContextValue {
  conversations: Conversation[] | null
  selectedId: string | null
  detail: ConversationDetail | null
  loading: boolean
  sending: boolean
  error: string | null
  pendingUserContent: string | null
  selectConversation: (id: string) => void
  createConversation: (input: { title?: string; platformId: string; model?: string }) => Promise<Conversation>
  sendMessage: (content: string) => Promise<void>
  refreshConversations: () => Promise<void>
  renameConversation: (id: string, title: string) => Promise<void>
  deleteConversation: (id: string) => Promise<void>
}

const ConversationContext = createContext<ConversationContextValue | null>(null)

export function useConversationContext(): ConversationContextValue {
  const ctx = useContext(ConversationContext)
  if (!ctx) {
    throw new Error('useConversationContext must be used within a ConversationProvider')
  }
  return ctx
}

// The one shared source of conversation state — replaces the two
// independent local-state copies ConversationPage and (step-17) the
// global widget would otherwise each hold. Mounted in
// AuthenticatedProviders, inside AuthProvider (see that file's own
// "future auth-dependent global provider" convention) so both
// consumers, wherever in the route tree they render, read the exact
// same conversations/selectedId/detail. See
// plan/ai/conversation/step-16-shared-conversation-context.md.
export function ConversationProvider({ children }: { children: ReactNode }) {
  const { isAuthenticated } = useAuth()
  const [conversations, setConversations] = useState<Conversation[] | null>(null)
  const [selectedId, setSelectedId] = useState<string | null>(null)
  const [detail, setDetail] = useState<ConversationDetail | null>(null)
  const [loading, setLoading] = useState(false)
  const [sending, setSending] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [pendingUserContent, setPendingUserContent] = useState<string | null>(null)
  const loadedRef = useRef(false)

  const refreshConversations = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      const list = await fetchConversations()
      setConversations(list)
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to load conversations.')
    } finally {
      setLoading(false)
    }
  }, [])

  // Eager-fetch the conversation list only (not any detail) once truly
  // authenticated — same gating ToolRegistryProvider already uses, and
  // the same "list only" scope the design settled on: enough for
  // step-17's widget to show something immediately, without changing
  // the full page's own "nothing selected on first load" behavior.
  useEffect(() => {
    if (!isAuthenticated || loadedRef.current) return
    loadedRef.current = true
    void refreshConversations()
  }, [isAuthenticated, refreshConversations])

  const selectConversation = useCallback((id: string) => {
    setSelectedId(id)
    setDetail(null)
    setError(null)
    fetchConversation(id)
      .then((d) => setDetail(d))
      .catch((err) => setError(err instanceof ApiError ? err.message : 'Failed to load conversation.'))
  }, [])

  const createConversation = useCallback(async (input: { title?: string; platformId: string; model?: string }) => {
    const created = await createConversationApi(input)
    setConversations((prev) => (prev ? [created, ...prev] : [created]))
    setSelectedId(created.id)
    setDetail({ ...created, messages: [] })
    return created
  }, [])

  // Mirrors ConversationPage.tsx's own established handleSend exactly:
  // always refetches detail afterward regardless of success/failure,
  // since a provider failure still records the user's own message
  // server-side — the real, refetched state is more truthful than an
  // optimistic one.
  const sendMessage = useCallback(
    async (content: string) => {
      if (!selectedId) return
      setError(null)
      setPendingUserContent(content)
      setSending(true)
      try {
        await sendMessageApi(selectedId, { content })
      } catch (err) {
        setError(err instanceof ApiError ? err.message : 'Failed to send message.')
      } finally {
        try {
          const d = await fetchConversation(selectedId)
          setDetail(d)
        } catch {
          // Best-effort refresh — a failure here doesn't need its own
          // error message on top of whatever the send itself surfaced.
        } finally {
          setPendingUserContent(null)
          setSending(false)
        }
      }
    },
    [selectedId],
  )

  const renameConversation = useCallback(async (id: string, title: string) => {
    const updated = await renameConversationApi(id, title)
    setConversations((prev) => (prev ? prev.map((c) => (c.id === id ? updated : c)) : prev))
    setDetail((prev) => (prev && prev.id === id ? { ...prev, title: updated.title } : prev))
  }, [])

  const deleteConversation = useCallback(
    async (id: string) => {
      await deleteConversationApi(id)
      setConversations((prev) => (prev ? prev.filter((c) => c.id !== id) : prev))
      if (selectedId === id) {
        setSelectedId(null)
        setDetail(null)
      }
    },
    [selectedId],
  )

  const value: ConversationContextValue = {
    conversations,
    selectedId,
    detail,
    loading,
    sending,
    error,
    pendingUserContent,
    selectConversation,
    createConversation,
    sendMessage,
    refreshConversations,
    renameConversation,
    deleteConversation,
  }

  return <ConversationContext.Provider value={value}>{children}</ConversationContext.Provider>
}
