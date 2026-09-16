import type { ActiveTurnSummary } from '../../api/conversations'
import { elapsedMsFromSummary } from './turnTiming'
import { formatDuration } from './formatDuration'

// TurnStatusBadge (step 28) — a small "still running" indicator for a
// conversation-list item, shared by ConversationSidebar.tsx and
// GlobalChatWidget.tsx's own ConversationList. Renders nothing when
// there's no running turn.
//
// Deliberately no independent per-item ticking interval — the elapsed
// value is recomputed at render time from whatever `conversations`
// state currently holds, which only actually changes once per
// LIST_POLL_INTERVAL_MS (ConversationContext.tsx). A live per-second
// timer running independently for every visible list row would be
// real, avoidable overhead for a background-awareness badge, where
// "still running, roughly how long" is the actual need — not
// second-accurate ticking (that's ThinkingIndicator's own job, for the
// one conversation actually open). See
// plan/ai/conversation/step-28-frontend-conversation-list-status-badges.md.
export function TurnStatusBadge({ activeTurn }: { activeTurn?: ActiveTurnSummary }) {
  if (activeTurn?.status !== 'running') return null

  const elapsedMs = elapsedMsFromSummary(activeTurn)

  return (
    <span className="inline-flex items-center gap-1 text-xs text-amber-700">
      <span className="h-1.5 w-1.5 animate-pulse rounded-full bg-amber-500" />
      Running{elapsedMs >= 1000 ? ` · ${formatDuration(elapsedMs)}` : ''}
    </span>
  )
}
