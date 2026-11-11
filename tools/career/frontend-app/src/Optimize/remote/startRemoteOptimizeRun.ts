import { createOptimizeRun, type OptimizeSubjectKind } from '../../api'
import { CoreApiError } from '../../Cinqo/Http/client'
import { createConversation, sendMessage, type SendMessageResult } from '../../Cinqo/Conversation/conversation'
import type { AiChoice } from '../../Cinqo/Platform/aiChoice'
import { optimizeConversationTitle } from '../optimizeRules'
import { buildOptimizePrompt } from './buildOptimizePrompt'
import { OPTIMIZE_RUN_TOOLS, RESTRICTION_NOT_APPLIED_MESSAGE, restrictionHolds } from './optimizeTools'

export interface StartedRemoteOptimizeRun {
  runId: string
  // The AI's turn: sent when the caller is ready to watch it; resolves with the model's last message.
  turn: () => Promise<SendMessageResult>
}

// The remote driver of "Optimize with AI" (plan 258, step 4): for a model that follows a prompt and calls tools. One hidden conversation, restricted to the run's two tools, one prompt.
// The AI's suggestions are stored by suggest_optimization as it goes; the run list, the reload and the watching of the turn stay with the caller. Rejects (with a readable message)
// when the conversation or the record fails.
export async function startRemoteOptimizeRun(
  subject: { kind: OptimizeSubjectKind; id: string; name: string },
  focus: string,
  ai: AiChoice,
): Promise<StartedRemoteOptimizeRun> {
  if (!ai.platform) throw new Error('No AI platform is available. Choose one in Settings.')
  const conversation = await createConversation({
    title: optimizeConversationTitle(subject.name),
    platformId: ai.platform.id,
    model: ai.model ?? undefined,
    hidden: true,
    // The AI may only use the tools this run needs: the server enforces it, not just the prompt.
    allowedTools: [...OPTIMIZE_RUN_TOOLS],
  }).catch((err: unknown) => {
    if (err instanceof CoreApiError && (err.status === 401 || err.status === 403)) {
      throw new Error('Ask an admin to grant you access to AI conversations.')
    }
    throw err
  })
  // The restriction must really be in place before the AI sees a single message.
  if (!restrictionHolds(conversation.allowedTools, OPTIMIZE_RUN_TOOLS)) {
    throw new Error(RESTRICTION_NOT_APPLIED_MESSAGE)
  }
  // Recorded before the first message, so a reload in between still has something to resume. A second open run of the subject is refused here (the server says so).
  const runId = await createOptimizeRun(subject.kind, subject.id, conversation.id, '', focus.trim())
  return {
    runId,
    turn: () => sendMessage(conversation.id, buildOptimizePrompt(subject, focus, runId)),
  }
}
