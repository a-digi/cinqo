import { useCallback, useEffect, useRef, useState } from 'react'
import { fetchCvAiRuns, finishCvAiRun, type CvAiRun } from '../api'
import { CoreApiError } from '../Cinqo/Http/client'
import type { SendMessageResult } from '../Cinqo/Conversation/conversation'
import type { AiChoice } from '../Cinqo/Platform/aiChoice'
import { NO_CV_SAVED_MESSAGE } from './cvAiRules'
import { resumeCvAiRun, startCvAiRun } from './cvAiRunner'

// The runs of one persona's "create a CV with AI" (step 115): the list, starting a run, and finishing
// it when the AI's turn ends. Same shape as usePersonaAiRuns: a run is a hidden conversation recorded
// BEFORE the first message so a reload can resume it; every run still generating is picked up again
// by waiting for its conversation's turn. The AI turn lives in the core app, so only this page can
// finish the run (the server ends a run nobody finishes after 30 minutes).
export function useCvAiRuns(personaId: string | null) {
  const [runs, setRuns] = useState<CvAiRun[]>([])
  const [error, setError] = useState('')
  // Runs this page is already waiting for, so none is watched twice.
  const watching = useRef(new Set<string>())
  const seq = useRef(0)

  const reload = useCallback(() => {
    if (!personaId) return Promise.resolve()
    const mine = ++seq.current
    return fetchCvAiRuns(personaId)
      .then((list) => {
        if (mine === seq.current) setRuns(list)
      })
      .catch((err: unknown) => {
        if (mine === seq.current) setError(err instanceof Error ? err.message : String(err))
      })
  }, [personaId])

  // Ends a run from what its turn produced: ready when the AI saved a CV (the server checks), else failed.
  const finish = useCallback(
    (runId: string, reply: SendMessageResult | undefined, err: unknown) => {
      let outcome: Promise<void>
      if (reply) {
        outcome = finishCvAiRun(runId, { status: 'ready' }).catch((putErr: unknown) =>
          // The AI saved nothing (the server refuses 'ready'): record that instead.
          finishCvAiRun(runId, { status: 'failed', error: NO_CV_SAVED_MESSAGE }).then(() => {
            console.error('the CV AI run ended without a CV', runId, putErr)
          }),
        )
      } else {
        const text =
          err instanceof CoreApiError && (err.status === 401 || err.status === 403)
            ? 'Ask an admin to grant you access to AI conversations.'
            : err instanceof Error
              ? err.message
              : 'The AI run failed.'
        outcome = finishCvAiRun(runId, { status: 'failed', error: text })
      }
      return outcome
        .catch((recordErr: unknown) => {
          // Already finished (another tab), or unreachable: the list below shows the truth.
          console.error('failed to record the CV AI run outcome', runId, recordErr)
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
  }, [personaId, reload])

  // Pick up every run that is still generating (after a reload).
  useEffect(() => {
    for (const run of runs) {
      if (run.status === 'generating') watch(run.id, resumeCvAiRun(run))
    }
  }, [runs, watch])

  // Starts a run (the driver is chosen in cvAiRunner). Rejects (with a readable message) when the conversation or the record fails.
  const start = useCallback(
    async (persona: { id: string; name: string }, focus: string, ai: AiChoice) => {
      const { runId, turn } = await startCvAiRun(persona, focus, ai)
      await reload()
      watch(runId, turn)
    },
    [reload, watch],
  )

  return { runs, error, start, reload }
}
