import { attachCvToRun, createCvAiRun, createCvDocument, fetchCvPersonaDefaults, fetchCvPhotoStatus, fetchCvTemplates } from '../../api'
import { CoreApiError } from '../../Cinqo/Http/client'
import { createConversation, sendMessage, type SendMessageResult } from '../../Cinqo/Conversation/conversation'
import type { AiChoice } from '../../Cinqo/Platform/aiChoice'
import { cvAiConversationTitle } from '../cvAiRules'
import { runLocalCv } from './runLocalCv'

export interface StartedLocalCvRun {
  runId: string
  turn: () => Promise<SendMessageResult>
}

// Starts a run for a model with the code driver. The hidden conversation is created with no tool list: the local platform offers a conversation without one no tools at all,
// so nothing the model says can call anything. The run is recorded before the first question, as for the remote driver. The turn is the whole code-driven run: it resolves
// with one plain sentence (the run then ends ready) and rejects with a readable reason (the run then ends failed).
export async function startLocalCvRun(persona: { id: string; name: string }, focus: string, ai: AiChoice): Promise<StartedLocalCvRun> {
  if (!ai.platform) throw new Error('No AI platform is available. Choose one in Settings.')
  const conversation = await createConversation({
    title: cvAiConversationTitle(persona.name),
    platformId: ai.platform.id,
    model: ai.model ?? undefined,
    hidden: true,
  }).catch((err: unknown) => {
    if (err instanceof CoreApiError && (err.status === 401 || err.status === 403)) {
      throw new Error('Ask an admin to grant you access to AI conversations.')
    }
    throw err
  })
  const runId = await createCvAiRun(persona.id, conversation.id, focus.trim())
  return {
    runId,
    turn: async () => {
      const result = await runLocalCv(
        {
          loadDefaults: fetchCvPersonaDefaults,
          loadTemplates: fetchCvTemplates,
          hasProfilePhoto: async (personaId) => (await fetchCvPhotoStatus(personaId)).hasPhoto,
          ask: async (prompt) => (await sendMessage(conversation.id, prompt, { fresh: true })).content,
          // With the profile's own default photo when the CV carries one (the server takes it from the profile; the model never names a file).
          createCv: (personaId, templateId, title, data, attachPhoto) => createCvDocument(personaId, templateId, title, data, attachPhoto),
          attach: attachCvToRun,
        },
        persona,
        focus,
        runId,
      )
      return { role: 'assistant', content: result.summary, createdAt: new Date().toISOString() }
    },
  }
}
