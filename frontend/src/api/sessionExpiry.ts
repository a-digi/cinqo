// A tiny, dependency-free pub/sub bridging the non-React fetch layer
// (api/client.ts) to React (AuthContext, frontend/auth/01). When a
// request's session is found to be irrecoverably dead — a 401 that
// survives the one renew attempt — the client emits here; AuthContext
// reacts by clearing auth state (which makes the mounted AuthGuards
// redirect to /login). Kept out of any component so client.ts, which is
// not a React module, can call it directly.

type Listener = () => void

const listeners = new Set<Listener>()

// Subscribe to session-expiry. Returns an unsubscribe function.
export function onSessionExpired(cb: Listener): () => void {
  listeners.add(cb)
  return () => {
    listeners.delete(cb)
  }
}

// Notify every subscriber that the session is dead. A module-level guard
// collapses a burst of simultaneously-failing requests into a single emit (reset
// on the next tick), so subscribers see one signal, not N.
let emitting = false

export function emitSessionExpired(): void {
  if (emitting) return
  emitting = true
  listeners.forEach((l) => l())
  setTimeout(() => {
    emitting = false
  }, 0)
}
