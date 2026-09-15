import { createContext, useCallback, useContext, useEffect, useRef, useState, type ReactNode } from 'react'
import { useAuth } from '../../Components/Auth/AuthContext'
import {
  fetchConversations,
  fetchConversation,
  fetchActiveTurn,
  stopActiveTurn,
  createConversation as createConversationApi,
  sendMessage as sendMessageApi,
  renameConversation as renameConversationApi,
  deleteConversation as deleteConversationApi,
  type Conversation,
  type ConversationDetail,
} from '../../api/conversations'
import { ApiError } from '../../api/client'

// How often to re-poll GET .../turns/active while a turn is running —
// plan/ai/conversation/step-23-detach-turn-execution-from-request.md.
// No existing polling convention anywhere else in this frontend to
// match; picked for a reasonably live-feeling log/elapsed display
// without hammering the backend.
const TURN_POLL_INTERVAL_MS = 2000

export interface ConversationContextValue {
  conversations: Conversation[] | null
  selectedId: string | null
  detail: ConversationDetail | null
  loading: boolean
  sending: boolean
  error: string | null
  pendingUserContent: string | null
  // The currently-running turn's own server-recorded start time (RFC3339)
  // and this client's own clock offset from the server (serverNow -
  // Date.now(), in ms) as of the most recent poll — null/0 while
  // `sending` is false. Lets ThinkingIndicator show real elapsed time
  // instead of restarting from 0 on every remount. See
  // plan/ai/conversation/step-24-server-tracked-turn-elapsed-time.md.
  turnStartedAt: string | null
  turnClockOffsetMs: number
  selectConversation: (id: string) => void
  createConversation: (input: { title?: string; platformId: string; model?: string }) => Promise<Conversation>
  sendMessage: (content: string) => Promise<void>
  // Requests cancellation of the selected conversation's own
  // currently-running turn — a no-op if nothing is running. Does not
  // itself flip `sending` off: the existing poll loop observes the
  // run's status turning "cancelled" and settles normally, the same
  // way it already handles "completed"/"failed". See
  // plan/ai/conversation/step-25-cancel-in-progress-turn.md.
  stopTurn: () => Promise<void>
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
  const [turnStartedAt, setTurnStartedAt] = useState<string | null>(null)
  const [turnClockOffsetMs, setTurnClockOffsetMs] = useState(0)
  const loadedRef = useRef(false)

  // pollGenerationRef guards a poll loop against acting once it's no
  // longer relevant — the user navigated to a different conversation,
  // or a second send started a fresh poll loop before an older one
  // noticed it should stop. Every new poll loop (and every
  // selectConversation call) increments this and checks it's still
  // the current value before ever touching state; the backend's own
  // detached run keeps going regardless — this only guards what the
  // UI does with the result.
  const pollGenerationRef = useRef(0)

  // pollUntilFinished polls GET .../turns/active every
  // TURN_POLL_INTERVAL_MS until the run leaves "running", then
  // refetches the full conversation for the real, finished message —
  // the async replacement for the old single-await sendMessageApi
  // call (plan/ai/conversation/step-23). Safe to call either right
  // after starting a new turn, or on mount/selection to resume
  // watching one already in progress from before the page was opened.
  const pollUntilFinished = useCallback(async (conversationId: string) => {
    const generation = ++pollGenerationRef.current
    setSending(true)
    for (;;) {
      let turn
      try {
        turn = await fetchActiveTurn(conversationId)
      } catch {
        // No turn has ever run for this conversation, or the lookup
        // itself failed — either way, nothing left to poll for.
        break
      }
      if (pollGenerationRef.current !== generation) return
      // Reconciled on every poll response, not just the first — cheap,
      // and keeps the client's own clock-offset estimate fresh for the
      // whole (potentially long) duration of a run.
      setTurnStartedAt(turn.startedAt)
      setTurnClockOffsetMs(Date.parse(turn.serverNow) - Date.now())
      if (turn.status !== 'running') break
      await new Promise((resolve) => setTimeout(resolve, TURN_POLL_INTERVAL_MS))
      if (pollGenerationRef.current !== generation) return
    }

    try {
      const d = await fetchConversation(conversationId)
      if (pollGenerationRef.current === generation) setDetail(d)
    } catch {
      // Best-effort refresh — matches the old sendMessage's own
      // "a failure here doesn't need its own error message" comment.
    } finally {
      if (pollGenerationRef.current === generation) {
        setPendingUserContent(null)
        setTurnStartedAt(null)
        setSending(false)
      }
    }
  }, [])

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

  const selectConversation = useCallback(
    (id: string) => {
      // Invalidate any poll loop still running for a previously
      // selected conversation — the backend's own detached run is
      // unaffected; this only stops this component from acting on its
      // result once the user has navigated away from it.
      pollGenerationRef.current++
      setSelectedId(id)
      setDetail(null)
      setError(null)
      setSending(false)
      setPendingUserContent(null)
      setTurnStartedAt(null)
      setTurnClockOffsetMs(0)
      fetchConversation(id)
        .then((d) => {
          setDetail(d)
          // A turn was already running when this page/tab opened (or
          // reopened) — resume watching it instead of leaving the UI
          // looking idle while the backend keeps working. See
          // plan/ai/conversation/step-23-detach-turn-execution-from-request.md.
          if (d.activeTurn?.status === 'running') {
            setPendingUserContent(d.activeTurn.userContent)
            // Set eagerly from this same response, rather than waiting
            // for pollUntilFinished's own first poll round trip — a
            // resumed turn should show correct elapsed time
            // immediately, not after one more network call.
            setTurnStartedAt(d.activeTurn.startedAt)
            setTurnClockOffsetMs(Date.parse(d.activeTurn.serverNow) - Date.now())
            void pollUntilFinished(id)
          }
        })
        .catch((err) => setError(err instanceof ApiError ? err.message : 'Failed to load conversation.'))
    },
    [pollUntilFinished],
  )

  const createConversation = useCallback(async (input: { title?: string; platformId: string; model?: string }) => {
    const created = await createConversationApi(input)
    setConversations((prev) => (prev ? [created, ...prev] : [created]))
    setSelectedId(created.id)
    setDetail({ ...created, messages: [] })
    return created
  }, [])

  // POST .../messages now only starts a detached turn run and returns
  // immediately (plan/ai/conversation/step-23) — the actual "wait for
  // the AI, then show the result" work happens in pollUntilFinished,
  // which keeps running (via its own setTimeout loop) independently of
  // this component's lifetime, the same way the backend's own run is
  // independent of this HTTP request's lifetime.
  const sendMessage = useCallback(
    async (content: string) => {
      if (!selectedId) return
      const conversationId = selectedId
      setError(null)
      setPendingUserContent(content)
      try {
        await sendMessageApi(conversationId, { content })
      } catch (err) {
        setError(err instanceof ApiError ? err.message : 'Failed to send message.')
        setPendingUserContent(null)
        return
      }
      await pollUntilFinished(conversationId)
    },
    [selectedId, pollUntilFinished],
  )

  // Fire-and-forget from the caller's own point of view: the stop
  // request itself is asynchronous server-side (see
  // stopActiveTurn's own doc comment) — the already-running
  // pollUntilFinished loop for this conversation is what actually
  // notices the run finishing (as "cancelled") and clears `sending`.
  // A failure here (e.g. the run already finished on its own a moment
  // before the click landed) is surfaced but otherwise harmless — the
  // poll loop settles normally either way.
  const stopTurn = useCallback(async () => {
    if (!selectedId) return
    try {
      await stopActiveTurn(selectedId)
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to stop the turn.')
    }
  }, [selectedId])

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
    turnStartedAt,
    turnClockOffsetMs,
    selectConversation,
    createConversation,
    sendMessage,
    stopTurn,
    refreshConversations,
    renameConversation,
    deleteConversation,
  }

  return <ConversationContext.Provider value={value}>{children}</ConversationContext.Provider>
}
