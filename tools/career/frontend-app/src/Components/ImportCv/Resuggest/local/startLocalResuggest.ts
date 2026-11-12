import { applyCVImportSuggestionChanges, fetchCVImportRun, fetchCvText } from '../../../../api'
import { CoreApiError } from '../../../../Cinqo/Http/client'
import { createConversation, sendMessage } from '../../../../Cinqo/Conversation/conversation'
import type { AiChoice } from '../../../../Cinqo/Platform/aiChoice'
import { parseProposal } from '../../parseProposal'
import { conversationTitle, type ResuggestScope } from '../resuggestRules'
import { runLocalResuggest, type LocalResuggestResult } from './runLocalResuggest'

export interface LocalResuggestChat {
  conversationId: string
  run: (wish: string, onProgress: (text: string) => void) => Promise<LocalResuggestResult>
}

// The local driver's chat (plan 268, step 4): a hidden conversation with NO tool list (the model is only asked small questions and calls nothing); each question is a fresh turn, so
// the model sees that question only. The changes go through the page's route, which keeps them inside the scope.
export async function startLocalResuggest(
  run: { id: string; mediaFileId: string; originalFilename: string },
  scope: ResuggestScope,
  ai: AiChoice,
): Promise<LocalResuggestChat> {
  if (!ai.platform) throw new Error('No AI platform is available. Choose one in Settings.')
  const conversation = await createConversation({
    title: conversationTitle(scope, run.originalFilename),
    platformId: ai.platform.id,
    model: ai.model ?? undefined,
    hidden: true,
  }).catch((err: unknown) => {
    if (err instanceof CoreApiError && (err.status === 401 || err.status === 403)) {
      throw new Error('Ask an admin to grant you access to AI conversations.')
    }
    throw err
  })
  return {
    conversationId: conversation.id,
    run: (wish, onProgress) =>
      runLocalResuggest(scope, wish, {
        readRun: async () => {
          const fresh = await fetchCVImportRun(run.id)
          if (!fresh) throw new Error('This import no longer exists.')
          return { proposal: parseProposal(fresh.aiProposal), revision: fresh.revision }
        },
        readCv: () => fetchCvText(run.mediaFileId),
        ask: async (prompt) => (await sendMessage(conversation.id, prompt, { fresh: true })).content,
        apply: async (ops, basedOn) => {
          await applyCVImportSuggestionChanges({ runId: run.id, section: scope.section, itemIds: scope.itemIds, changes: ops, basedOn })
        },
        progress: onProgress,
      }),
  }
}
