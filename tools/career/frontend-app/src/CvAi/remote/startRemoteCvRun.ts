import { createCvAiRun } from '../../api'
import { CoreApiError } from '../../Cinqo/Http/client'
import { createConversation, sendMessage, type SendMessageResult } from '../../Cinqo/Conversation/conversation'
import type { AiChoice } from '../../Cinqo/Platform/aiChoice'
import { cvAiConversationTitle } from '../cvAiRules'
import { buildCvAiPrompt } from './buildCvAiPrompt'
import { CV_AI_RUN_TOOLS, RESTRICTION_NOT_APPLIED_MESSAGE, restrictionHolds } from './cvAiTools'

// The remote driver of "Create a CV with AI" (step 115; split out in plan 253): for a model that follows a long prompt and calls tools well. One hidden conversation,
// restricted to the run's tools, one prompt that walks the model through reading the persona, choosing a design, rendering, making the PDF and saving. This is the code
// that used to be the body of useCvAiRuns.start, unchanged; the run list, the reload and the watching of the turn stay with the caller.
export interface StartedCvRun {
  runId: string
  // The AI's turn: sent when the caller is ready to watch it; resolves with the model's last message.
  turn: () => Promise<SendMessageResult>
}

// Starts a run. Rejects (with a readable message) when the conversation or the record fails.
export async function startRemoteCvRun(persona: { id: string; name: string }, focus: string, ai: AiChoice): Promise<StartedCvRun> {
  if (!ai.platform) throw new Error('No AI platform is available. Choose one in Settings.')
  const conversation = await createConversation({
    title: cvAiConversationTitle(persona.name),
    platformId: ai.platform.id,
    model: ai.model ?? undefined,
    hidden: true,
    // The AI may only use the tools this run needs: the server enforces it, not just the prompt.
    allowedTools: [...CV_AI_RUN_TOOLS],
  }).catch((err: unknown) => {
    if (err instanceof CoreApiError && (err.status === 401 || err.status === 403)) {
      throw new Error('Ask an admin to grant you access to AI conversations.')
    }
    throw err
  })
  // The restriction must really be in place before the AI sees a single message.
  if (!restrictionHolds(conversation.allowedTools, CV_AI_RUN_TOOLS)) {
    throw new Error(RESTRICTION_NOT_APPLIED_MESSAGE)
  }
  // Recorded before the first message, so a reload in between still has something to resume.
  const runId = await createCvAiRun(persona.id, conversation.id, focus.trim())
  return { runId, turn: () => sendMessage(conversation.id, buildCvAiPrompt(persona, focus, runId)) }
}
