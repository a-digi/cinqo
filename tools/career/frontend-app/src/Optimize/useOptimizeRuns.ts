import { useCallback, useEffect, useRef, useState } from 'react'
import {
  decideOptimizeSuggestions,
  deleteOptimizeRun,
  fetchOptimizeRuns,
  finishOptimizeRun,
  type OptimizeDecided,
  type OptimizeDecision,
  type OptimizeRun,
  type OptimizeSubjectKind,
} from '../api'
import { CoreApiError } from '../Cinqo/Http/client'
import type { SendMessageResult } from '../Cinqo/Conversation/conversation'
import type { AiChoice } from '../Cinqo/Platform/aiChoice'
import { NOTHING_TO_IMPROVE } from './optimizeRules'
import { resumeOptimizeRun, startOptimizeRun } from './optimizeRunner'

// The runs of one subject's "Optimize with AI" (plan 258): the list, starting a run, finishing it when the AI's turn ends, and the person's decisions. Same shape as useCvAiRuns: a run
// is recorded BEFORE the first message so a reload can resume it; every run still generating is picked up again by waiting for its turn. The AI turn lives in the core app (or in
// this page, for a code-driven run), so only this page can finish the run (the server ends a run nobody finishes after 30 minutes).
export function useOptimizeRuns(subjectKind: OptimizeSubjectKind, subjectId: string | null) {
  const [runs, setRuns] = useState<OptimizeRun[]>([])
  const [error, setError] = useState('')
  // One line about what just happened that leaves no run behind ("nothing to improve"); the person dismisses it.
  const [notice, setNotice] = useState('')
  const watching = useRef(new Set<string>())
  const seq = useRef(0)

  const reload = useCallback(() => {
    if (!subjectId) return Promise.resolve()
    const mine = ++seq.current
    return fetchOptimizeRuns(subjectKind, subjectId)
      .then((list) => {
        if (mine === seq.current) setRuns(list)
      })
      .catch((err: unknown) => {
        if (mine === seq.current) setError(err instanceof Error ? err.message : String(err))
      })
  }, [subjectKind, subjectId])

  // Ends a run from what its turn produced: ready when the turn ended (the server says how many suggestions wait), else failed.
  const finish = useCallback(
    (runId: string, reply: SendMessageResult | undefined, err: unknown) => {
      let outcome: Promise<void>
      if (reply) {
        outcome = finishOptimizeRun(runId, { status: 'ready' }).then((waiting) => {
          if (waiting === 0) setNotice(NOTHING_TO_IMPROVE)
        })
      } else {
        const text =
          err instanceof CoreApiError && (err.status === 401 || err.status === 403)
            ? 'Ask an admin to grant you access to AI conversations.'
            : err instanceof Error
              ? err.message
              : 'The AI run failed.'
        outcome = finishOptimizeRun(runId, { status: 'failed', error: text }).then(() => undefined)
      }
      return outcome
        .catch((recordErr: unknown) => {
          // Already finished (another tab), or unreachable: the list below shows the truth.
          console.error('failed to record the optimization outcome', runId, recordErr)
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
    setNotice('')
    void reload()
  }, [subjectKind, subjectId, reload])

  // Pick up every run that is still generating (after a reload).
  useEffect(() => {
    for (const run of runs) {
      if (run.status === 'generating') watch(run.id, resumeOptimizeRun(run))
    }
  }, [runs, watch])

  // Starts a run. Rejects (with a readable message) when the conversation or the record fails.
  const start = useCallback(
    async (subject: { kind: OptimizeSubjectKind; id: string; name: string }, focus: string, ai: AiChoice) => {
      setNotice('')
      const { runId, turn } = await startOptimizeRun(subject, focus, ai)
      await reload()
      watch(runId, turn)
    },
    [reload, watch],
  )

  // The person's decisions on the suggestions of a run; the list is read again afterwards (a run whose last suggestion was decided is gone).
  const decide = useCallback(
    async (runId: string, decisions: OptimizeDecision[]): Promise<OptimizeDecided> => {
      const decided = await decideOptimizeSuggestions(runId, decisions)
      await reload()
      return decided
    },
    [reload],
  )

  // Dismisses a run (a failed one, or a review the person does not want).
  const dismiss = useCallback(
    async (runId: string) => {
      await deleteOptimizeRun(runId)
      await reload()
    },
    [reload],
  )

  return {
    runs,
    error,
    notice,
    clearNotice: () => {
      setNotice('')
    },
    start,
    decide,
    dismiss,
    reload,
  }
}
