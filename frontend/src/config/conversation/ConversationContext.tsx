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
  type SubAgentRun,
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
  // promptTokens/completionTokens/totalTokens (step 34) — reconciled
  // on every poll alongside turnStartedAt/turnClockOffsetMs, from the
  // same fetchActiveTurn response. 0 until the first iteration's
  // response has actually landed.
  promptTokens: number
  completionTokens: number
  totalTokens: number
  // subAgents (step 41 — Sub Agents) — reconciled on every poll, same
  // as promptTokens/completionTokens/totalTokens above, straight from
  // the same fetchActiveTurn response.
  subAgents: SubAgentRun[]
  // mainReplyReady (step 41 follow-up — sub-agents outlive the reply)
  // — false while the orchestrator's own reply is still in flight,
  // true from the moment it's been fetched into `detail`. Sending/
  // ThinkingIndicator should stop showing once this flips (the real
  // reply already exists as its own message) — but this watch entry
  // stays alive, and subAgents keeps being polled, until every
  // sub-agent it lists has ALSO left "running": since step 41's own
  // "the reply returns early" change, a turn's own sub-agents can keep
  // working well after the turn itself finishes, and this is what
  // keeps that visible live instead of freezing into a stale snapshot
  // the moment the reply appears.
  mainReplyReady: boolean
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

// Context + hook co-located deliberately (same convention as every other
// *Context.tsx in this codebase) — costs this file Fast Refresh for the
// hook specifically, never a runtime issue.
// eslint-disable-next-line react-refresh/only-export-components
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
  // TURN_POLL_INTERVAL_MS until the run AND every sub-agent it spawned
  // have both left "running" — the per-conversation-ID replacement for
  // the old single-scalar pollUntilFinished (plan/ai/conversation/step-23).
  // Safe to call either right after starting a new turn, or on
  // selection to resume watching one already in progress. Runs to
  // completion independently of any other conversation's own watchTurn
  // call, and independently of whether conversationId stays selected
  // for its whole duration.
  //
  // Two-phase since step 41's own "the reply returns early" backend
  // change: the turn itself can finish (and its real reply become
  // fetchable) well before its own sub-agents do. mainReplyFetched
  // marks the moment that first happens — the conversation is refetched
  // ONCE right then (so the real reply appears immediately, not delayed
  // by however much longer any sub-agent takes), and mainReplyReady is
  // set on the watch entry so sending/ThinkingIndicator stop showing a
  // now-redundant "thinking" placeholder — but the loop itself keeps
  // going, still polling subAgents, until every one of them has ALSO
  // left "running". Only then is the watch entry actually removed.
  const watchTurn = useCallback(async (conversationId: string) => {
    const myToken = (watchTokensRef.current[conversationId] ?? 0) + 1
    watchTokensRef.current[conversationId] = myToken
    setTurnWatches((prev) => ({
      ...prev,
      [conversationId]: {
        // `noUncheckedIndexedAccess` isn't on, so TS types prev[conversationId]
        // as always-defined — it genuinely isn't here (this can be the very
        // first watch for this id), so the optional chain stays.
        // eslint-disable-next-line @typescript-eslint/no-unnecessary-condition
        pendingUserContent: prev[conversationId]?.pendingUserContent ?? null,
        turnStartedAt: null,
        turnClockOffsetMs: 0,
        promptTokens: 0,
        completionTokens: 0,
        totalTokens: 0,
        subAgents: [],
        mainReplyReady: false,
      },
    }))

    let mainReplyFetched = false

    const fetchMainReply = async () => {
      try {
        const d = await fetchConversation(conversationId)
        if (watchTokensRef.current[conversationId] === myToken) {
          // Only overwrites the open detail pane if the user is still
          // actually looking at this conversation — a background watch
          // finishing must not clobber whatever conversation is
          // currently selected.
          setDetail((prev) => (prev?.id === conversationId ? d : prev))
        }
      } catch {
        // Best-effort refresh — matches this codebase's own established
        // "a failure here doesn't need its own error message" convention.
      }
    }

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
        // Same noUncheckedIndexedAccess caveat as above — this entry can
        // genuinely be gone by the time a poll response comes back (e.g.
        // deleteConversation ran concurrently).
        // eslint-disable-next-line @typescript-eslint/no-unnecessary-condition
        prev[conversationId]
          ? {
              ...prev,
              [conversationId]: {
                ...prev[conversationId],
                // step 32 — reconciled on every poll, not just seeded
                // once at watch-start: a watch resumed after a page
                // reload has no other way to recover the user's own
                // just-sent prompt (sendMessage's own optimistic seed,
                // above, only ever existed in that now-gone page's
                // memory), but fetchActiveTurn already returns it
                // every single poll (turn_runs.user_content, saved
                // synchronously before the async turn even starts).
                // See plan/ai/conversation/step-32-restore-pending-
                // user-message-after-reload.md.
                pendingUserContent: turn.userContent,
                turnStartedAt: turn.startedAt,
                turnClockOffsetMs: Date.parse(turn.serverNow) - Date.now(),
                // step 34 — same reasoning as pendingUserContent just
                // above: reconciled on every poll from the same
                // fetchActiveTurn response, since turn_runs' own
                // columns are already updated live, once per tool-loop
                // iteration (AddTokenUsage), not only once the turn
                // finishes.
                promptTokens: turn.promptTokens,
                completionTokens: turn.completionTokens,
                totalTokens: turn.totalTokens,
                subAgents: turn.subAgents ?? [],
              },
            }
          : prev,
      )

      const subAgentsStillRunning = (turn.subAgents ?? []).some((a) => a.status === 'running')

      if (turn.status !== 'running' && !mainReplyFetched) {
        mainReplyFetched = true
        await fetchMainReply()
        if (watchTokensRef.current[conversationId] !== myToken) return // superseded
        setTurnWatches((prev) =>
          // eslint-disable-next-line @typescript-eslint/no-unnecessary-condition
          prev[conversationId] ? { ...prev, [conversationId]: { ...prev[conversationId], mainReplyReady: true } } : prev,
        )
      }

      if (turn.status !== 'running' && !subAgentsStillRunning) break
      await new Promise((resolve) => setTimeout(resolve, TURN_POLL_INTERVAL_MS))
      if (watchTokensRef.current[conversationId] !== myToken) return // superseded
    }

    // Reached once the turn AND every sub-agent it spawned have both
    // left "running" — or the poll loop broke early on a lookup
    // failure, in which case mainReplyFetched may still be false and
    // this is the same one-shot best-effort refresh watchTurn always
    // did before sub-agents could outlive the turn itself.
    if (!mainReplyFetched) {
      await fetchMainReply()
    }
    if (watchTokensRef.current[conversationId] === myToken) {
      setTurnWatches((prev) => {
        if (!(conversationId in prev)) return prev
        const next = { ...prev }
        Reflect.deleteProperty(next, conversationId)
        return next
      })
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
    return () => {
      clearInterval(id)
    }
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
        .catch((err: unknown) => {
          setError(err instanceof ApiError ? err.message : 'Failed to load conversation.')
        })
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
    setDetail({
      id: created.id,
      title: created.title,
      startedAt: created.startedAt,
      platformId: created.platformId,
      model: created.model,
      messages: [],
    })
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
        setTurnWatches((prev) => ({
          ...prev,
          [conversationId]: {
            pendingUserContent: content,
            turnStartedAt: null,
            turnClockOffsetMs: 0,
            promptTokens: 0,
            completionTokens: 0,
            totalTokens: 0,
            subAgents: [],
            mainReplyReady: false,
          },
        }))
      }
      try {
        await sendMessageApi(conversationId, { content })
      } catch (err) {
        setError(err instanceof ApiError ? err.message : 'Failed to send message.')
        if (!alreadyWatching) {
          setTurnWatches((prev) => {
            if (!(conversationId in prev)) return prev
            const next = { ...prev }
            Reflect.deleteProperty(next, conversationId)
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
    setDetail((prev) => (prev?.id === id ? { ...prev, title: updated.title } : prev))
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
        Reflect.deleteProperty(next, id)
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
