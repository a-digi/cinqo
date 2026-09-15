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

// How often to re-fetch the whole conversation list — looser than
// TURN_POLL_INTERVAL_MS since this now runs continuously for the
// entire authenticated session against every conversation at once
// (to keep every item's own activeTurn badge live), not just while one
// turn is actively being watched. See
// plan/ai/conversation/step-27-frontend-periodic-list-refresh.md.
const LIST_POLL_INTERVAL_MS = 8000

// TurnWatch is one conversation's own live turn-tracking state —
// plan/ai/conversation/step-29-frontend-per-conversation-turn-watch-registry.md.
// Presence of a conversation's own ID as a key in turnWatches IS "this
// conversation is currently being watched" — there's no separate
// boolean to let drift out of sync with the map's own membership.
export interface TurnWatch {
  pendingUserContent: string | null
  turnStartedAt: string | null
  turnClockOffsetMs: number
}

export interface ConversationContextValue {
  conversations: Conversation[] | null
  selectedId: string | null
  detail: ConversationDetail | null
  loading: boolean
  error: string | null
  // Every conversation currently being watched (tier 2 — full detail,
  // not just the list-level badge tier), keyed by conversation ID.
  // Replaces the old single-scalar sending/pendingUserContent/
  // turnStartedAt/turnClockOffsetMs: those only ever tracked "whichever
  // conversation is selected," so sending in one conversation and
  // switching to another used to silently stop watching the first
  // one's turn (it kept running server-side regardless — only the UI
  // lost track of it). This registry lets any number of conversations'
  // turns be watched concurrently and correctly, independent of which
  // one is currently selected. See
  // plan/ai/conversation/step-29-frontend-per-conversation-turn-watch-registry.md.
  turnWatches: Record<string, TurnWatch>
  selectConversation: (id: string) => void
  createConversation: (input: { title?: string; platformId: string; model?: string }) => Promise<Conversation>
  // Explicit conversationId (not implicitly "the selected one") — the
  // literal shape multi-conversation infrastructure requires: a caller
  // can now start a turn in a conversation that isn't currently
  // selected. Both existing consumers always pass their own
  // `selectedId` today; this is a widening of the surface, not a
  // behavior change for them.
  sendMessage: (conversationId: string, content: string) => Promise<void>
  // Requests cancellation of conversationId's own currently-running
  // turn — a no-op if nothing is running. Does not itself remove the
  // conversation from turnWatches: the already-running watch loop
  // observes the run's status turning "cancelled" and settles
  // normally, the same way it already handles "completed"/"failed".
  // See plan/ai/conversation/step-25-cancel-in-progress-turn.md.
  stopTurn: (conversationId: string) => Promise<void>
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
  const [error, setError] = useState<string | null>(null)
  const [turnWatches, setTurnWatches] = useState<Record<string, TurnWatch>>({})

  // watchTokensRef guards each conversation's own watch loop
  // independently — replaces the old single, app-wide
  // pollGenerationRef. A new watchTurn(id) call issues a fresh token
  // for that id and stores it; the loop checks its own token is still
  // current before touching state, so a second, superseding watch for
  // the SAME id cleanly wins — but a watch for a DIFFERENT id is never
  // affected, unlike the old single-ref design. See
  // plan/ai/conversation/step-29-frontend-per-conversation-turn-watch-registry.md.
  const watchTokensRef = useRef<Record<string, number>>({})

  // watchTurn polls GET .../turns/active for conversationId every
  // TURN_POLL_INTERVAL_MS until the run leaves "running", then
  // refetches the full conversation for the real, finished message —
  // the per-conversation-ID replacement for the old single-scalar
  // pollUntilFinished (plan/ai/conversation/step-23). Safe to call
  // either right after starting a new turn, or on selection to resume
  // watching one already in progress. Runs to completion independently
  // of any other conversation's own watchTurn call, and independently
  // of whether conversationId stays selected for its whole duration.
  const watchTurn = useCallback(async (conversationId: string) => {
    const myToken = (watchTokensRef.current[conversationId] ?? 0) + 1
    watchTokensRef.current[conversationId] = myToken
    setTurnWatches((prev) => ({
      ...prev,
      [conversationId]: {
        pendingUserContent: prev[conversationId]?.pendingUserContent ?? null,
        turnStartedAt: null,
        turnClockOffsetMs: 0,
      },
    }))

    for (;;) {
      let turn
      try {
        turn = await fetchActiveTurn(conversationId)
      } catch {
        // No turn has ever run for this conversation, or the lookup
        // itself failed — either way, nothing left to poll for.
        break
      }
      if (watchTokensRef.current[conversationId] !== myToken) return // superseded
      // Reconciled on every poll response, not just the first — cheap,
      // and keeps the client's own clock-offset estimate fresh for the
      // whole (potentially long) duration of a run.
      setTurnWatches((prev) =>
        prev[conversationId]
          ? { ...prev, [conversationId]: { ...prev[conversationId], turnStartedAt: turn.startedAt, turnClockOffsetMs: Date.parse(turn.serverNow) - Date.now() } }
          : prev,
      )
      if (turn.status !== 'running') break
      await new Promise((resolve) => setTimeout(resolve, TURN_POLL_INTERVAL_MS))
      if (watchTokensRef.current[conversationId] !== myToken) return // superseded
    }

    try {
      const d = await fetchConversation(conversationId)
      if (watchTokensRef.current[conversationId] === myToken) {
        // Only overwrites the open detail pane if the user is still
        // actually looking at this conversation — a background watch
        // finishing must not clobber whatever conversation is
        // currently selected.
        setDetail((prev) => (prev && prev.id === conversationId ? d : prev))
      }
    } catch {
      // Best-effort refresh — matches this codebase's own established
      // "a failure here doesn't need its own error message" convention.
    } finally {
      if (watchTokensRef.current[conversationId] === myToken) {
        setTurnWatches((prev) => {
          if (!(conversationId in prev)) return prev
          const next = { ...prev }
          delete next[conversationId]
          return next
        })
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

  // Background refresh (step 27) — keeps every conversation's own
  // activeTurn badge live (step-26/28) without flashing the full-page
  // `loading` state, or surfacing a transient failure via `error`, on
  // every tick the way the user-facing refreshConversations above
  // does; a failed tick is silently skipped and the next one tries
  // again. See plan/ai/conversation/step-27-frontend-periodic-list-refresh.md.
  const refreshConversationsSilently = useCallback(async () => {
    try {
      const list = await fetchConversations()
      setConversations(list)
    } catch {
      // best-effort — next interval tick tries again.
    }
  }, [])

  // Eager-fetch the conversation list once truly authenticated (same
  // gating ToolRegistryProvider already uses), then keep it live for
  // the rest of the session via a periodic silent refresh. See
  // plan/ai/conversation/step-27-frontend-periodic-list-refresh.md.
  useEffect(() => {
    if (!isAuthenticated) return
    void refreshConversations()
    const id = setInterval(() => {
      void refreshConversationsSilently()
    }, LIST_POLL_INTERVAL_MS)
    return () => clearInterval(id)
  }, [isAuthenticated, refreshConversations, refreshConversationsSilently])

  // turnWatchesRef mirrors turnWatches so a plain membership check
  // (turnWatchExists, used by selectConversation below) doesn't need
  // turnWatches itself as a dependency — which would otherwise change
  // selectConversation's identity on every watch start/stop.
  const turnWatchesRef = useRef(turnWatches)
  turnWatchesRef.current = turnWatches
  const turnWatchExists = useCallback((id: string) => id in turnWatchesRef.current, [])

  // No longer cancels any other conversation's own watch — switching
  // away from a conversation whose turn is still running leaves that
  // watch running untouched (turnWatches[id] stays populated);
  // switching back to it later finds live data already waiting, no
  // "resuming" needed. See
  // plan/ai/conversation/step-29-frontend-per-conversation-turn-watch-registry.md.
  const selectConversation = useCallback(
    (id: string) => {
      setSelectedId(id)
      setDetail(null)
      setError(null)
      fetchConversation(id)
        .then((d) => {
          setDetail(d)
          // A turn was already running when this page/tab opened (or
          // reopened) — resume watching it instead of leaving the UI
          // looking idle while the backend keeps working, unless it's
          // already being watched (e.g. this conversation's own turn
          // was started from here a moment ago and is still in
          // flight). See
          // plan/ai/conversation/step-23-detach-turn-execution-from-request.md.
          if (d.activeTurn?.status === 'running' && !turnWatchExists(id)) {
            void watchTurn(id)
          }
        })
        .catch((err) => setError(err instanceof ApiError ? err.message : 'Failed to load conversation.'))
    },
    [watchTurn, turnWatchExists],
  )

  const createConversation = useCallback(async (input: { title?: string; platformId: string; model?: string }) => {
    const created = await createConversationApi(input)
    setConversations((prev) => (prev ? [created, ...prev] : [created]))
    setSelectedId(created.id)
    // A freshly created conversation can never already have a turn
    // running — created.activeTurn is always undefined here (the
    // create endpoint's own response never sets it), so this is never
    // actually dropping live data, just satisfying ConversationDetail's
    // own fuller ActiveTurn type (step-26/27) instead of the lean
    // ActiveTurnSummary Conversation itself now carries.
    setDetail({ id: created.id, title: created.title, startedAt: created.startedAt, platformId: created.platformId, model: created.model, messages: [] })
    return created
  }, [])

  // POST .../messages now only starts a detached turn run and returns
  // immediately (plan/ai/conversation/step-23) — the actual "wait for
  // the AI, then show the result" work happens in watchTurn, which
  // keeps running (via its own setTimeout loop) independently of this
  // component's lifetime and of whichever conversation is currently
  // selected, the same way the backend's own run is independent of
  // this HTTP request's lifetime.
  const sendMessage = useCallback(
    async (conversationId: string, content: string) => {
      setError(null)
      // If a watch already exists for this conversation (a turn is
      // already running there — this call is about to fail with a 409
      // from the backend's own turn_runs_one_running_idx), leave
      // turnWatches alone entirely: don't optimistically add an entry
      // that wasn't there, and don't delete the real, still-running
      // watch on failure below. Only the common case (no existing
      // watch) gets the optimistic pendingUserContent entry.
      const alreadyWatching = turnWatchExists(conversationId)
      if (!alreadyWatching) {
        setTurnWatches((prev) => ({ ...prev, [conversationId]: { pendingUserContent: content, turnStartedAt: null, turnClockOffsetMs: 0 } }))
      }
      try {
        await sendMessageApi(conversationId, { content })
      } catch (err) {
        setError(err instanceof ApiError ? err.message : 'Failed to send message.')
        if (!alreadyWatching) {
          setTurnWatches((prev) => {
            if (!(conversationId in prev)) return prev
            const next = { ...prev }
            delete next[conversationId]
            return next
          })
        }
        return
      }
      await watchTurn(conversationId)
    },
    [watchTurn, turnWatchExists],
  )

  // Fire-and-forget from the caller's own point of view: the stop
  // request itself is asynchronous server-side (see
  // stopActiveTurn's own doc comment) — the already-running watchTurn
  // loop for conversationId is what actually notices the run finishing
  // (as "cancelled") and clears it from turnWatches. A failure here
  // (e.g. the run already finished on its own a moment before the
  // click landed) is surfaced but otherwise harmless — the watch loop
  // settles normally either way.
  const stopTurn = useCallback(async (conversationId: string) => {
    try {
      await stopActiveTurn(conversationId)
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to stop the turn.')
    }
  }, [])

  const renameConversation = useCallback(async (id: string, title: string) => {
    const updated = await renameConversationApi(id, title)
    setConversations((prev) => (prev ? prev.map((c) => (c.id === id ? updated : c)) : prev))
    setDetail((prev) => (prev && prev.id === id ? { ...prev, title: updated.title } : prev))
  }, [])

  const deleteConversation = useCallback(
    async (id: string) => {
      await deleteConversationApi(id)
      // Invalidate any in-flight watch for the now-deleted conversation
      // — its next fetchActiveTurn call would 404 and break out of the
      // loop on its own regardless, but doing it here removes any
      // stale entry from turnWatches immediately rather than waiting
      // for that next poll tick.
      watchTokensRef.current[id] = (watchTokensRef.current[id] ?? 0) + 1
      setTurnWatches((prev) => {
        if (!(id in prev)) return prev
        const next = { ...prev }
        delete next[id]
        return next
      })
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
    error,
    turnWatches,
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
