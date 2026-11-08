// formatDuration.ts (step 21) — extracted from MessageThread.tsx
// (step 14/15's own completed-turn duration display) since
// ThinkingIndicator.tsx now needs the exact same formatting for its
// own live, ticking elapsed timer. Extended with an hours branch —
// a completed turn's own durationMs is never hours long in practice,
// but a live wait genuinely can be. ms is always a whole number of
// milliseconds — formatted here, never on the backend, matching this
// app's existing "backend returns raw values, frontend formats for
// display" split. See
// plan/ai/conversation/step-21-thinking-indicator-timer.md.
export function formatDuration(ms: number): string {
  if (ms < 1000) return `${ms}ms`
  const totalSeconds = Math.floor(ms / 1000)
  const hours = Math.floor(totalSeconds / 3600)
  const minutes = Math.floor((totalSeconds % 3600) / 60)
  const seconds = totalSeconds % 60
  if (hours > 0) return `${hours}h ${minutes}m ${seconds}s`
  if (minutes > 0) return `${minutes}m ${seconds}s`
  return `${(ms / 1000).toFixed(1)}s`
}
