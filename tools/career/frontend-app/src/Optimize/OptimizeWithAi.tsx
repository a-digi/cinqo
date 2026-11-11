import { useState } from 'react'
import type { OptimizeSubjectKind } from '../api'
import type { AiChoice } from '../Cinqo/Platform/aiChoice'
import { resolveAiOverride, type AiOverride } from '../Cinqo/Platform/aiOverride'
import type { Platform } from '../Cinqo/Platform/platformRepository'
import { CvAiPicker } from '../CvAi/CvAiPicker'
import { SETTINGS_PATH } from '../paths'
import { SparkleIcon } from '../Shared/Icons/icons'
import { inputClass } from '../Shared/inputClass'
import { Modal } from '../Shared/Modal/Modal'
import { OptimizeReview } from './OptimizeReview'
import { approvedCount, optimizeView, reviewLabel } from './optimizeRules'
import { useOptimizeRuns } from './useOptimizeRuns'

// The "Optimize with AI" button of a profile or of a target role, its dialog and the review of what the AI suggested (plan 258). The dialog starts from the Career AI settings (`ai`)
// and lets the person pick another platform and model for this run only (the same picker as "Create CV with AI"); the pick is not saved. The button says what is going on: working,
// suggestions waiting for the review, a failed run (with Dismiss). onApplied is called after at least one suggestion was approved, so the page reads its texts again.
export function OptimizeWithAi({
  subjectKind,
  subjectId,
  subjectName,
  ai,
  platforms,
  onApplied,
}: {
  subjectKind: OptimizeSubjectKind
  subjectId: string
  subjectName: string
  ai: AiChoice
  platforms: Platform[]
  onApplied: () => void
}) {
  const runs = useOptimizeRuns(subjectKind, subjectId)
  const state = optimizeView(runs.runs)
  const [dialogOpen, setDialogOpen] = useState(false)
  const [reviewOpen, setReviewOpen] = useState(false)
  const [focus, setFocus] = useState('')
  const [starting, setStarting] = useState(false)
  const [error, setError] = useState('')
  const [override, setOverride] = useState<AiOverride | null>(null)
  const choice = resolveAiOverride({ platform: ai.platform, model: ai.model }, platforms, override)
  const word = subjectKind === 'profile' ? 'profile' : 'target role'
  const aiText = choice.platform ? `${choice.platform.name}${choice.model ? ` · ${choice.model}` : ''}` : ''

  function closeDialog() {
    if (starting) return
    setDialogOpen(false)
    setError('')
    setOverride(null)
  }

  function start() {
    setError('')
    setStarting(true)
    runs
      .start({ kind: subjectKind, id: subjectId, name: subjectName }, focus.trim(), {
        platform: choice.platform,
        model: choice.model,
        source: ai.source,
      })
      .then(() => {
        setDialogOpen(false)
        setFocus('')
        setOverride(null)
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
      .finally(() => {
        setStarting(false)
      })
  }

  const disabledReason =
    state.view === 'working'
      ? 'The AI is already working on this.'
      : state.view === 'review'
        ? 'Review the suggestions first.'
        : !ai.platform
          ? 'No AI platform is available. Choose one in Settings.'
          : ''

  return (
    <div data-testid="optimize-with-ai" className="flex flex-col items-end gap-1.5">
      <div className="flex flex-wrap items-center justify-end gap-2">
        {state.view === 'review' ? (
          <button
            type="button"
            onClick={() => {
              setReviewOpen(true)
            }}
            className="flex shrink-0 items-center gap-1.5 rounded-md bg-purple-700 px-3.5 py-2 text-sm font-medium text-white transition-colors hover:bg-purple-800"
          >
            <SparkleIcon />
            {reviewLabel(state.count)}
          </button>
        ) : (
          <button
            type="button"
            disabled={disabledReason !== ''}
            title={disabledReason || `Let the AI suggest better wording for this ${word}'s texts`}
            onClick={() => {
              setDialogOpen(true)
            }}
            className="flex shrink-0 items-center gap-1.5 rounded-md border border-purple-200 bg-white px-3.5 py-2 text-sm font-medium text-purple-700 transition-colors hover:bg-purple-50 disabled:cursor-not-allowed disabled:opacity-50"
          >
            <SparkleIcon />
            {state.view === 'working' ? 'Optimizing…' : 'Optimize with AI'}
          </button>
        )}
      </div>
      {state.view === 'review' && state.staleCount > 0 && (
        <p className="text-xs text-amber-700">
          {state.staleCount === 1 ? '1 suggestion is out of date' : `${String(state.staleCount)} suggestions are out of date`}: its text
          changed.
        </p>
      )}
      {state.view === 'failed' && (
        <p role="alert" className="max-w-sm text-right text-xs text-red-700">
          The optimization failed: {state.run.error ?? 'no reason was recorded'}{' '}
          <button
            type="button"
            onClick={() => {
              void runs.dismiss(state.run.id)
            }}
            className="underline"
          >
            Dismiss
          </button>
        </p>
      )}
      {runs.notice && (
        <p role="status" className="max-w-sm text-right text-xs text-gray-600">
          {runs.notice}{' '}
          <button type="button" onClick={runs.clearNotice} className="underline">
            OK
          </button>
        </p>
      )}

      <Modal open={dialogOpen} title="Optimize with AI" onClose={closeDialog}>
        <p className="mb-3 text-sm text-gray-600">
          The AI reads the texts of the {word} <span className="font-medium text-gray-800">{subjectName}</span> and suggests better wording.
          It never adds anything that is not already there, and nothing changes until you approve a suggestion; a declined suggestion is
          deleted.
        </p>
        <label className="block text-xs font-medium text-gray-500">
          What should it focus on? (optional)
          <textarea
            value={focus}
            onChange={(e) => {
              setFocus(e.target.value)
            }}
            maxLength={500}
            rows={3}
            placeholder="e.g. shorter and more concrete, or for data roles"
            className={`mt-1 ${inputClass}`}
          />
        </label>
        {platforms.length > 0 && (
          <>
            <p className="mt-4 text-xs font-medium text-gray-500">AI for this run</p>
            <CvAiPicker
              choice={choice}
              settings={{ platform: ai.platform, model: ai.model }}
              platforms={platforms}
              onChange={(platformId, model) => {
                setOverride({ platformId, model })
              }}
              onReset={() => {
                setOverride(null)
              }}
            />
          </>
        )}
        <p className="mt-3 text-xs text-gray-500">
          The texts of this {word} are sent to <span className="font-medium text-gray-700">{aiText || 'the AI platform'}</span> to do this.
          {platforms.length === 0 && (
            <>
              {' '}
              <button
                type="button"
                onClick={() => {
                  window.__cinqoToolBridge.navigate(SETTINGS_PATH)
                }}
                className="text-blue-700 underline"
              >
                Change it in Settings
              </button>
            </>
          )}
        </p>
        {error && (
          <p role="alert" className="mt-3 text-sm text-red-700">
            {error}
          </p>
        )}
        <div className="mt-5 flex justify-end gap-2">
          <button
            type="button"
            onClick={closeDialog}
            disabled={starting}
            className="rounded-md border border-gray-200 px-3.5 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50 disabled:opacity-50"
          >
            Cancel
          </button>
          <button
            type="button"
            onClick={start}
            disabled={starting || !choice.platform}
            className="flex items-center gap-1.5 rounded-md bg-gray-900 px-3.5 py-2 text-sm font-medium text-white hover:bg-gray-800 disabled:opacity-50"
          >
            <SparkleIcon />
            {starting ? 'Starting…' : 'Start'}
          </button>
        </div>
      </Modal>

      {state.view === 'review' && (
        <OptimizeReview
          open={reviewOpen}
          run={state.run}
          subjectName={subjectName}
          onClose={() => {
            setReviewOpen(false)
          }}
          onDecide={async (decisions) => {
            const decided = await runs.decide(state.run.id, decisions)
            if (approvedCount(decisions, decided) > 0) onApplied()
            return decided
          }}
        />
      )}
    </div>
  )
}
