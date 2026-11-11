// optimizeRunner.ts — the one place that starts and resumes the AI turn of an "Optimize with AI" run (plan 258). The drivers live in their own folders and never import each other; this file
// is the only one that imports them. A model the platform marks with the code driver (`drivers[model] === 'code'`, Phi-4-mini today) cannot follow a prompt with tools, so code does
// the work and asks it one text at a time (local/); every other model takes the one-prompt-with-tools path (remote/).
import type { OptimizeRun, OptimizeSubjectKind } from '../api'
import { awaitTurnCompletion, type SendMessageResult } from '../Cinqo/Conversation/conversation'
import type { AiChoice } from '../Cinqo/Platform/aiChoice'
import { usesCodeDriver } from '../Cinqo/Platform/modelLabels'
import { startLocalOptimizeRun } from './local/startLocalOptimizeRun'
import { startRemoteOptimizeRun } from './remote/startRemoteOptimizeRun'

export interface StartedOptimizeRun {
  runId: string
  // The AI's turn: sent when the caller is ready to watch it; resolves when it is over and rejects with a readable reason when it failed.
  turn: () => Promise<SendMessageResult>
}

export type OptimizeDriver = 'local' | 'remote'

export function optimizeDriverFor(ai: AiChoice): OptimizeDriver {
  return ai.platform && usesCodeDriver(ai.platform, ai.model) ? 'local' : 'remote'
}

export interface OptimizeProgress {
  done: number
  total: number
}

// Starts a run for a profile or a persona. Rejects (with a readable message) when the conversation or the record fails. onProgress is told how far a code-driven run is (a tool conversation
// has no such count).
export function startOptimizeRun(
  subject: { kind: OptimizeSubjectKind; id: string; name: string },
  focus: string,
  ai: AiChoice,
  onProgress?: (p: OptimizeProgress) => void,
): Promise<StartedOptimizeRun> {
  return optimizeDriverFor(ai) === 'local'
    ? startLocalOptimizeRun(subject, focus, ai, onProgress)
    : startRemoteOptimizeRun(subject, focus, ai)
}

export const RUN_LOST_MESSAGE = 'The page was closed before the AI finished. Start it again.'

// The turn to watch for a run that is still generating when the page opens (after a reload). A tool conversation goes on in the core app, so its turn is waited for; a code-driven
// run lived in the page that was closed and cannot be resumed: it ends as failed.
export function resumeOptimizeRun(run: Pick<OptimizeRun, 'driver' | 'conversationId'>): () => Promise<SendMessageResult> {
  return async () => {
    if (run.driver === 'local' || run.conversationId === '') throw new Error(RUN_LOST_MESSAGE)
    return awaitTurnCompletion(run.conversationId)
  }
}
