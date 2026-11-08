// turnTiming.ts (step 28) — extracted from ThinkingIndicator.tsx (step
// 24's own clock-skew-corrected elapsed-time formula) so
// TurnStatusBadge can compute the same value from an ActiveTurnSummary
// without duplicating the formula. See
// plan/ai/conversation/step-28-frontend-conversation-list-status-badges.md.
import type { ActiveTurnSummary } from '../../api/conversations'

// effectiveStartMs is startedAt (RFC3339, the server's own recorded
// turn-start time) shifted by clockOffsetMs (serverNow - Date.now(),
// as of the most recent poll) so that Date.now() - effectiveStartMs
// always equals real server-side elapsed time, even when the viewer's
// local clock disagrees with the server's. See
// plan/ai/conversation/step-24-server-tracked-turn-elapsed-time.md.
export function effectiveStartMs(startedAt: string, clockOffsetMs: number): number {
  return Date.parse(startedAt) - clockOffsetMs
}

// elapsedMsFromSummary computes a one-shot elapsed value directly from
// an ActiveTurnSummary (startedAt + serverNow) — for a render-time
// snapshot (e.g. a list badge) rather than a live-ticking timer, which
// instead tracks clockOffsetMs across polls itself (see
// ThinkingIndicator.tsx).
export function elapsedMsFromSummary(summary: Pick<ActiveTurnSummary, 'startedAt' | 'serverNow'>): number {
  const clockOffsetMs = Date.parse(summary.serverNow) - Date.now()
  return Math.max(0, Date.now() - effectiveStartMs(summary.startedAt, clockOffsetMs))
}
