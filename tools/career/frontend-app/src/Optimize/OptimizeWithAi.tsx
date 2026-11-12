import { useEffect, useRef, useState } from 'react'
import type { OptimizeRun, OptimizeSubjectKind } from '../api'
import type { AiChoice } from '../Cinqo/Platform/aiChoice'
import { resolveAiOverride, type AiOverride } from '../Cinqo/Platform/aiOverride'
import type { Platform } from '../Cinqo/Platform/platformRepository'
import { CvAiPicker } from '../CvAi/CvAiPicker'
import { SETTINGS_PATH } from '../paths'
import { SparkleIcon } from '../Shared/Icons/icons'
import { inputClass } from '../Shared/inputClass'
import { Modal } from '../Shared/Modal/Modal'
import { CHIP_FADE_MS, CHIP_SHOW_MS, appliedText } from './motion'
import { OptimizeReview } from './OptimizeReview'
import { approvedCount, optimizeView, reviewLabel } from './optimizeRules'
import { useOptimizeRuns } from './useOptimizeRuns'

// The "Optimize with AI" button of a profile or of a target role, its dialog and the review of what the AI suggested (plan 258). The dialog starts from the Career AI settings (`ai`)
// and lets the person pick another platform and model for this run only (the same picker as "Create CV with AI"); the pick is not saved. The button says what is going on: working,
// suggestions waiting for the review, a failed run (with Dismiss). onApplied is called after at least one suggestion was approved, so the page reads its texts again.
// Step 7: the working sign pulses (with a progress bar for a code-driven run), the button changes with a fade, the review button pops in, and a chip says how many changes were applied.
const FOCUS_RING = 'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-purple-400 focus-visible:ring-offset-1'

export function OptimizeWithAi({
  subjectKind,
  subjectId,
  subjectName,
  ai,
  platforms,
  onApplied,
  variant = 'button',
}: {
  subjectKind: OptimizeSubjectKind
  subjectId: string
  subjectName: string
  ai: AiChoice
  platforms: Platform[]
  onApplied: () => void
  // 'button' (the default) is the button with words of the profile's and the target role's own page; 'icon' is the compact round icon of a card in a list (plan 258, step 8): the same
  // dialog, the same review, the same runs; the state is in the look of the icon, its tooltip and its label, and the lines under the button become a small bubble under the icon.
  variant?: 'button' | 'icon'
}) {
  const runs = useOptimizeRuns(subjectKind, subjectId)
  const state = optimizeView(runs.runs)
  const [dialogOpen, setDialogOpen] = useState(false)
  const [reviewOpen, setReviewOpen] = useState(false)
  // The review gives the focus back to the button that opened it, but that button is replaced when the last suggestion is decided ("Review suggestions" becomes "Optimize with AI"):
  // once the review has closed, the focus goes to the main button of the panel.
  const panel = useRef<HTMLDivElement>(null)
  const reviewWasOpen = useRef(false)
  useEffect(() => {
    if (reviewWasOpen.current && !reviewOpen) panel.current?.querySelector<HTMLButtonElement>('button:not(:disabled)')?.focus()
    reviewWasOpen.current = reviewOpen
  }, [reviewOpen])
  const [focus, setFocus] = useState('')
  const [starting, setStarting] = useState(false)
  const [error, setError] = useState('')
  const [override, setOverride] = useState<AiOverride | null>(null)
  // "3 changes applied": shown for a few seconds after approvals, the last moment fading out.
  const [applied, setApplied] = useState<{ count: number; fading: boolean } | null>(null)
  const chipTimers = useRef<number[]>([])
  useEffect(
    () => () => {
      chipTimers.current.forEach((t) => {
        window.clearTimeout(t)
      })
    },
    [],
  )
  function showApplied(count: number) {
    chipTimers.current.forEach((t) => {
      window.clearTimeout(t)
    })
    setApplied((prev) => ({ count: (prev?.count ?? 0) + count, fading: false }))
    chipTimers.current = [
      window.setTimeout(() => {
        setApplied((prev) => (prev ? { ...prev, fading: true } : prev))
      }, CHIP_SHOW_MS - CHIP_FADE_MS),
      window.setTimeout(() => {
        setApplied(null)
      }, CHIP_SHOW_MS),
    ]
  }
  // The review stays on screen after its run is gone (its last cards are leaving, then "All suggestions decided" shows): the last run it showed is kept, without suggestions.
  const lastRun = useRef<OptimizeRun | null>(null)
  if (state.view === 'review') lastRun.current = state.run
  const reviewRun: OptimizeRun | null =
    state.view === 'review' ? state.run : lastRun.current ? { ...lastRun.current, suggestions: [] } : null
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

  const iconMode = variant === 'icon'
  const progressText = runs.progress && runs.progress.total > 0 ? ` ${String(runs.progress.done)} of ${String(runs.progress.total)}` : ''
  const iconTitle =
    state.view === 'working'
      ? `Optimizing ${subjectName}…${progressText}`
      : state.view === 'review'
        ? `${reviewLabel(state.count)}${state.staleCount > 0 ? ` · ${String(state.staleCount)} out of date` : ''}`
        : state.view === 'failed'
          ? `The last optimization failed: ${state.run.error ?? 'no reason was recorded'}`
          : disabledReason || `Optimize ${subjectName} with AI`
  const iconLabel =
    state.view === 'working'
      ? `Optimizing ${subjectName}${progressText ? ` (${progressText.trim()})` : ''}`
      : state.view === 'review'
        ? `${reviewLabel(state.count)} for ${subjectName}`
        : state.view === 'failed'
          ? `Optimize ${subjectName} with AI (the last run failed)`
          : `Optimize ${subjectName} with AI`
  const RING = 2 * Math.PI * 14
  const iconFace = (
    <>
      <button
        key={state.view === 'review' ? 'review' : 'start'}
        type="button"
        disabled={state.view !== 'review' && disabledReason !== ''}
        title={iconTitle}
        aria-label={iconLabel}
        onClick={() => {
          if (state.view === 'review') setReviewOpen(true)
          else setDialogOpen(true)
        }}
        className={`relative flex h-8 w-8 shrink-0 items-center justify-center rounded-full transition-colors disabled:cursor-not-allowed disabled:opacity-50 ${FOCUS_RING} ${
          state.view === 'review'
            ? 'opt-pop opt-lift bg-purple-700 text-white hover:bg-purple-800'
            : `${state.view === 'working' ? 'opt-working' : 'opt-lift'} border border-purple-200 bg-white text-purple-700 hover:bg-purple-50`
        }`}
      >
        <span className={state.view === 'working' ? 'opt-icon-pulse' : 'inline-flex'}>
          <SparkleIcon />
        </span>
        {state.view === 'working' && runs.progress && runs.progress.total > 0 && (
          <span
            role="progressbar"
            aria-label="Optimization progress"
            aria-valuemin={0}
            aria-valuemax={runs.progress.total}
            aria-valuenow={runs.progress.done}
            className="pointer-events-none absolute inset-0"
          >
            <svg viewBox="0 0 32 32" className="h-full w-full -rotate-90" aria-hidden="true">
              <circle
                className="opt-ring"
                cx="16"
                cy="16"
                r="14"
                fill="none"
                stroke="currentColor"
                strokeWidth="2"
                strokeLinecap="round"
                strokeDasharray={RING}
                strokeDashoffset={RING * (1 - runs.progress.done / runs.progress.total)}
              />
            </svg>
          </span>
        )}
        {state.view === 'review' && (
          <span
            data-testid="optimize-badge"
            aria-hidden="true"
            className="opt-pop absolute -right-1 -top-1 flex h-4 min-w-4 items-center justify-center rounded-full bg-amber-500 px-1 text-[10px] font-bold leading-none text-white"
          >
            {state.count}
          </span>
        )}
        {state.view === 'failed' && (
          <span
            data-testid="optimize-failed-dot"
            aria-hidden="true"
            className="absolute -right-0.5 -top-0.5 h-2.5 w-2.5 rounded-full border border-white bg-red-500"
          />
        )}
      </button>
      {(applied !== null || runs.notice !== '') && (
        <div className="absolute right-0 top-full z-10 mt-1 flex w-max max-w-[16rem] flex-col items-end gap-1">
          {applied && (
            <p
              role="status"
              className={`opt-pop rounded-full bg-green-50 px-2.5 py-0.5 text-xs font-medium text-green-700 shadow-sm ${applied.fading ? 'opt-fade-out' : ''}`}
            >
              {appliedText(applied.count)}
            </p>
          )}
          {runs.notice && (
            <p
              role="status"
              className="opt-fade-in rounded-md bg-white px-2 py-1 text-right text-xs text-gray-600 shadow-md ring-1 ring-gray-200"
            >
              {runs.notice}{' '}
              <button type="button" onClick={runs.clearNotice} className="underline">
                OK
              </button>
            </p>
          )}
        </div>
      )}
    </>
  )
  const buttonFace = (
    <>
      <div className="flex flex-wrap items-center justify-end gap-2">
        {state.view === 'review' ? (
          <button
            key="review"
            type="button"
            onClick={() => {
              setReviewOpen(true)
            }}
            className={`opt-pop opt-lift flex shrink-0 items-center gap-1.5 rounded-md bg-purple-700 px-3.5 py-2 text-sm font-medium text-white transition-colors hover:bg-purple-800 ${FOCUS_RING}`}
          >
            <SparkleIcon />
            {reviewLabel(state.count)}
          </button>
        ) : (
          <button
            key="start"
            type="button"
            disabled={disabledReason !== ''}
            title={disabledReason || `Let the AI suggest better wording for this ${word}'s texts`}
            onClick={() => {
              setDialogOpen(true)
            }}
            className={`${state.view === 'working' ? 'opt-working' : 'opt-lift'} flex shrink-0 items-center gap-1.5 rounded-md border border-purple-200 bg-white px-3.5 py-2 text-sm font-medium text-purple-700 transition-colors hover:bg-purple-50 disabled:cursor-not-allowed disabled:opacity-50 ${FOCUS_RING}`}
          >
            <span className={state.view === 'working' ? 'opt-icon-pulse' : 'inline-flex'}>
              <SparkleIcon />
            </span>
            <span key={`${state.view}-${String(runs.progress?.done ?? '')}`} className="opt-fade-in">
              {state.view === 'working'
                ? runs.progress && runs.progress.total > 0
                  ? `Optimizing… ${String(runs.progress.done)} of ${String(runs.progress.total)}`
                  : 'Optimizing…'
                : 'Optimize with AI'}
            </span>
          </button>
        )}
      </div>
      {state.view === 'working' && runs.progress && runs.progress.total > 0 && (
        <div
          role="progressbar"
          aria-label="Optimization progress"
          aria-valuemin={0}
          aria-valuemax={runs.progress.total}
          aria-valuenow={runs.progress.done}
          className="h-1 w-full overflow-hidden rounded-full bg-purple-100"
        >
          <div
            className="opt-bar h-full rounded-full bg-purple-500"
            style={{ width: `${String(Math.round((runs.progress.done / runs.progress.total) * 100))}%` }}
          />
        </div>
      )}
      {applied && (
        <p
          role="status"
          className={`opt-pop rounded-full bg-green-50 px-2.5 py-0.5 text-xs font-medium text-green-700 ${applied.fading ? 'opt-fade-out' : ''}`}
        >
          {appliedText(applied.count)}
        </p>
      )}
      {state.view === 'review' && state.staleCount > 0 && (
        <p className="opt-fade-in text-xs text-amber-700">
          {state.staleCount === 1 ? '1 suggestion is out of date' : `${String(state.staleCount)} suggestions are out of date`}: its text
          changed.
        </p>
      )}
      {state.view === 'failed' && (
        <p role="alert" className="opt-fade-in max-w-sm text-right text-xs text-red-700">
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
        <p role="status" className="opt-fade-in max-w-sm text-right text-xs text-gray-600">
          {runs.notice}{' '}
          <button type="button" onClick={runs.clearNotice} className="underline">
            OK
          </button>
        </p>
      )}
    </>
  )

  return (
    <div
      ref={panel}
      data-testid="optimize-with-ai"
      data-variant={variant}
      className={iconMode ? 'relative inline-flex' : 'flex flex-col items-end gap-1.5'}
    >
      {iconMode ? iconFace : buttonFace}
      <Modal open={dialogOpen} title="Optimize with AI" onClose={closeDialog}>
        {iconMode && state.view === 'failed' && (
          <p role="alert" className="opt-fade-in mb-3 rounded-md bg-red-50 px-3 py-2 text-sm text-red-700">
            The last optimization failed: {state.run.error ?? 'no reason was recorded'}{' '}
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
            {starting ? <span className="opt-spin" aria-hidden="true" /> : <SparkleIcon />}
            {starting ? 'Starting…' : 'Start'}
          </button>
        </div>
      </Modal>

      {reviewOpen && reviewRun && (
        <OptimizeReview
          open
          run={reviewRun}
          subjectName={subjectName}
          onClose={() => {
            setReviewOpen(false)
          }}
          onDecide={async (decisions) => {
            const decided = await runs.decide(reviewRun.id, decisions)
            const approved = approvedCount(decisions, decided)
            if (approved > 0) {
              showApplied(approved)
              onApplied()
            }
            return decided
          }}
        />
      )}
    </div>
  )
}
