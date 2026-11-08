// followTurn.ts — what the open chat does when it looks at the latest turn of its conversation (plan/ai/small-llm-local/step-210).
//
// A page follows a turn it started (watchTurn), or one that was already running when it opened. A turn that ANOTHER client starts later in the same
// conversation — a tool that drives a conversation one step at a time, the Career import — was invisible: the chat showed the turns up to the one it had
// watched and stayed there while the work went on. So the chat looks at the conversation's latest turn every few seconds and decides with this.

export type FollowAction = 'watch' | 'reload' | 'none'

// The latest turn of the conversation (null when it has none), the id of the turn the chat last took care of, and whether a watch is already running:
//  - a running turn that nobody watches is watched (the watch shows it live and loads the conversation when it ends);
//  - a turn that has finished and is not the one the chat last took care of means messages the chat does not have: reload;
//  - anything else needs nothing.
export function followAction(latest: { turnRunId: string; status: string } | null, lastSeenTurnId: string | undefined, watching: boolean): FollowAction {
  if (latest === null) return 'none'
  if (latest.status === 'running') return watching ? 'none' : 'watch'
  return latest.turnRunId === lastSeenTurnId ? 'none' : 'reload'
}

// How often the chat looks. A step of an import takes seconds to minutes; this keeps the chat within a few seconds of it without a request storm.
export const FOLLOW_INTERVAL_MS = 4000
