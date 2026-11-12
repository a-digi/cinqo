import { deleteConversation, createConversation, sendMessage, type SendMessageResult } from '../../../Cinqo/Conversation/conversation'
import { CoreApiError } from '../../../Cinqo/Http/client'
import type { AiChoice } from '../../../Cinqo/Platform/aiChoice'
import { recordCVImportSectionChat } from '../../../api'
import { RESTRICTION_NOT_APPLIED_MESSAGE, restrictionHolds } from '../../../PersonaAi/personaTools'
import { buildFollowUp, buildResuggestPrompt } from './buildResuggestPrompt'
import { conversationTitle, type ResuggestScope } from './resuggestRules'
import { SECTION_CHAT_TOOLS } from './resuggestTools'

export interface ResuggestChat {
  conversationId: string
  // The first message (with the rules and the scope), then the next ones; each resolves with the model's last message when its turn ends.
  first: (wish: string) => Promise<SendMessageResult>
  next: (wish: string) => Promise<SendMessageResult>
}

// Starts the chat of one dialog (plan 268): a hidden conversation restricted to the chat's tools, then the scope recorded on the server, and only then is a message sent. When the
// scope cannot be recorded the conversation is removed again and the reason is thrown: without the binding the AI would not be held inside its section.
export async function startResuggestChat(
  run: { id: string; mediaFileId: string; originalFilename: string },
  scope: ResuggestScope,
  ai: AiChoice,
): Promise<ResuggestChat> {
  if (!ai.platform) throw new Error('No AI platform is available. Choose one in Settings.')
  const conversation = await createConversation({
    title: conversationTitle(scope, run.originalFilename),
    platformId: ai.platform.id,
    model: ai.model ?? undefined,
    hidden: true,
    allowedTools: [...SECTION_CHAT_TOOLS],
  }).catch((err: unknown) => {
    if (err instanceof CoreApiError && (err.status === 401 || err.status === 403)) {
      throw new Error('Ask an admin to grant you access to AI conversations.')
    }
    throw err
  })
  if (!restrictionHolds(conversation.allowedTools, SECTION_CHAT_TOOLS)) {
    await deleteConversation(conversation.id).catch(() => undefined)
    throw new Error(RESTRICTION_NOT_APPLIED_MESSAGE)
  }
  try {
    await recordCVImportSectionChat({ runId: run.id, conversationId: conversation.id, section: scope.section, itemIds: scope.itemIds })
  } catch (err) {
    await deleteConversation(conversation.id).catch(() => undefined)
    throw err
  }
  return {
    conversationId: conversation.id,
    first: (wish) => sendMessage(conversation.id, buildResuggestPrompt({ runId: run.id, mediaFileId: run.mediaFileId, scope, wish })),
    next: (wish) => sendMessage(conversation.id, buildFollowUp(scope, wish)),
  }
}
