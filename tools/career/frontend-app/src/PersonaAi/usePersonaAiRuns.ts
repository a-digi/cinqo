import { useCallback, useEffect, useRef, useState } from 'react'
import { createPersonaAiRun, fetchPersonaAiRuns, finishPersonaAiRun, type PersonaAiRun } from '../api'
import { CoreApiError } from '../Cinqo/Http/client'
import { awaitTurnCompletion, createConversation, sendMessage, type SendMessageResult } from '../Cinqo/Conversation/conversation'
import type { AiChoice } from '../Cinqo/Platform/aiChoice'
import { buildPersonaPrompt, personaAiConversationTitle } from './buildPersonaPrompt'
import { PERSONA_RUN_TOOLS, RESTRICTION_NOT_APPLIED_MESSAGE, restrictionHolds } from './personaTools'

// The runs of one profile's "create personas with AI" (step 112): the list, starting a run, and
// finishing it when the AI's turn ends. A run is a hidden conversation (like Job Match), recorded
// BEFORE the first message is sent so a reload can resume it; after a reload every run that is
// still generating is picked up again by waiting for its conversation's turn. The AI turn lives in
// the core app, so only this page can finish the run (the server ends a run that nobody finishes
// after 30 minutes).
export function usePersonaAiRuns(profileId: string | null) {
  const [runs, setRuns] = useState<PersonaAiRun[]>([])
  const [error, setError] = useState('')
  // Runs this page is already waiting for, so none is watched twice.
  const watching = useRef(new Set<string>())
  const seq = useRef(0)

  const reload = useCallback(() => {
    if (!profileId) return Promise.resolve()
    const mine = ++seq.current
    return fetchPersonaAiRuns(profileId)
      .then((list) => {
        if (mine === seq.current) setRuns(list)
      })
      .catch((err: unknown) => {
        if (mine === seq.current) setError(err instanceof Error ? err.message : String(err))
      })
  }, [profileId])

  // Ends a run from what its turn produced: the reply, or the error that ended it.
  const finish = useCallback(
    (runId: string, reply: SendMessageResult | undefined, err: unknown) => {
      let outcome: Promise<void>
      if (reply) {
        // The AI handed its suggestions over with suggest_persona; its last message is only text.
        outcome = finishPersonaAiRun(runId, { status: 'ready' }).catch((putErr: unknown) =>
          finishPersonaAiRun(runId, {
            status: 'failed',
            error: `The suggestions were refused: ${putErr instanceof Error ? putErr.message : String(putErr)}`,
          }),
        )
      } else {
        const text =
          err instanceof CoreApiError && (err.status === 401 || err.status === 403)
            ? 'Ask an admin to grant you access to AI conversations.'
            : err instanceof Error
              ? err.message
              : 'The AI run failed.'
        outcome = finishPersonaAiRun(runId, { status: 'failed', error: text })
      }
      return outcome
        .catch((recordErr: unknown) => {
          // Already finished (another tab), or unreachable: the list below shows the truth.
          console.error('failed to record the persona AI run outcome', runId, recordErr)
        })
        .then(() => {
          watching.current.delete(runId)
          return reload()
        })
    },
    [reload],
  )

  const watch = useCallback(
    (runId: string, turn: () => Promise<SendMessageResult>) => {
      if (watching.current.has(runId)) return
      watching.current.add(runId)
      turn().then(
        (reply) => finish(runId, reply, undefined),
        (err: unknown) => finish(runId, undefined, err),
      )
    },
    [finish],
  )

  useEffect(() => {
    setRuns([])
    setError('')
    void reload()
  }, [profileId, reload])

  // Pick up every run that is still generating (after a reload, or when another run exists).
  useEffect(() => {
    for (const run of runs) {
      if (run.status === 'generating') watch(run.id, () => awaitTurnCompletion(run.conversationId))
    }
  }, [runs, watch])

  // Starts a run. Rejects (with a readable message) when the conversation or the record fails.
  const start = useCallback(
    async (profile: { id: string; name: string }, focus: string, ai: AiChoice) => {
      if (!ai.platform) throw new Error('No AI platform is available. Choose one in Settings.')
      const conversation = await createConversation({
        title: personaAiConversationTitle(profile.name),
        platformId: ai.platform.id,
        model: ai.model ?? undefined,
        hidden: true,
        // The AI only reads in this run (step 113): the server enforces it, not just the prompt.
        allowedTools: [...PERSONA_RUN_TOOLS],
      }).catch((err: unknown) => {
        // The same friendly message the other AI flows give a person without AI access.
        if (err instanceof CoreApiError && (err.status === 401 || err.status === 403)) {
          throw new Error('Ask an admin to grant you access to AI conversations.')
        }
        throw err
      })
      // The restriction must really be in place before the AI sees a single message: a server that
      // does not know it would ignore the field and leave the AI free to write.
      if (!restrictionHolds(conversation.allowedTools, PERSONA_RUN_TOOLS)) {
        throw new Error(RESTRICTION_NOT_APPLIED_MESSAGE)
      }
      // Recorded before the first message, so a reload in between still has something to resume.
      const runId = await createPersonaAiRun(profile.id, conversation.id, focus.trim())
      await reload()
      watch(runId, () => sendMessage(conversation.id, buildPersonaPrompt(profile, focus, runId)))
    },
    [reload, watch],
  )

  return { runs, error, start, reload }
}
