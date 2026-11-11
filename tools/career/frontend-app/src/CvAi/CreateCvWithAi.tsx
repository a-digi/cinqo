import { useEffect, useState } from 'react'
import { fetchCvPhotoStatus } from '../api'
import type { AiChoice } from '../Cinqo/Platform/aiChoice'
import { resolveAiOverride, type AiOverride } from '../Cinqo/Platform/aiOverride'
import type { Platform } from '../Cinqo/Platform/platformRepository'
import { SparkleIcon } from '../Shared/Icons/icons'
import { inputClass } from '../Shared/inputClass'
import { Modal } from '../Shared/Modal/Modal'
import { SETTINGS_PATH } from '../paths'
import { CvAiPicker } from './CvAiPicker'
import { decidePhoto } from './photoDecision'

// The "Create CV with AI" button and its dialog (step 115): the optional note on what the CV is for,
// the AI that will be used, and one line about the data going to it. The button says why it is
// disabled. The manual "New CV" next to it is untouched.
// Plan 252: the dialog starts from the Career AI settings (`ai`) and lets the person pick another platform and model for this run only (from `platforms`, the
// usable ones); the pick is not saved and not remembered, and onStart gets the AI in effect. With no usable platform there is no picker (and no AI to start with).
export function CreateCvWithAi({
  disabledReason,
  ai,
  platforms,
  personaId,
  personaName,
  onStart,
}: {
  // Empty when the button can be used.
  disabledReason: string
  ai: AiChoice
  platforms: Platform[]
  // The target role, to look up whether its profile has a photo (plan 256).
  personaId: string
  personaName: string
  onStart: (focus: string, choice: AiChoice) => Promise<void>
}) {
  const [open, setOpen] = useState(false)
  const [focus, setFocus] = useState('')
  const [starting, setStarting] = useState(false)
  const [error, setError] = useState('')
  const [override, setOverride] = useState<AiOverride | null>(null)
  const usable = platforms
  // plan 256: whether the profile has a photo, looked up when the dialog opens (null until known or when it cannot be looked up).
  const [hasPhoto, setHasPhoto] = useState<boolean | null>(null)
  useEffect(() => {
    if (!open) return
    let cancelled = false
    setHasPhoto(null)
    fetchCvPhotoStatus(personaId).then(
      (status) => {
        if (!cancelled) setHasPhoto(status.hasPhoto)
      },
      () => {
        if (!cancelled) setHasPhoto(null)
      },
    )
    return () => {
      cancelled = true
    }
  }, [open, personaId])
  const choice = resolveAiOverride({ platform: ai.platform, model: ai.model }, usable, override)

  function close() {
    if (starting) return
    setOpen(false)
    setError('')
    setOverride(null)
  }

  function start() {
    setError('')
    setStarting(true)
    onStart(focus, { platform: choice.platform, model: choice.model, source: ai.source })
      .then(() => {
        setOpen(false)
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

  // What the CV will carry, said before Start (plan 256): the same for every AI, since the page makes the CV for both drivers.
  const photoDecision = hasPhoto === null ? null : decidePhoto(hasPhoto, focus)
  const photoLine =
    !choice.platform || photoDecision === null
      ? ''
      : photoDecision.reason === 'photo'
        ? 'The CV includes your profile photo. Write "without photo" in the note to leave it out.'
        : photoDecision.reason === 'asked-none'
          ? 'The CV will have no photo, as you asked.'
          : 'Your profile has no photo, so the CV has none.'

  const aiText = choice.platform ? `${choice.platform.name}${choice.model ? ` · ${choice.model}` : ''}` : ''

  return (
    <>
      <button
        type="button"
        disabled={disabledReason !== ''}
        title={disabledReason || 'Let the AI choose a design and create a CV from this target role'}
        onClick={() => {
          setOpen(true)
        }}
        className="flex shrink-0 items-center gap-1.5 rounded-md border border-purple-200 bg-white px-3.5 py-2 text-sm font-medium text-purple-700 transition-colors hover:bg-purple-50 disabled:cursor-not-allowed disabled:opacity-50"
      >
        <SparkleIcon />
        Create CV with AI
      </button>

      <Modal open={open} title="Create a CV with AI" onClose={close}>
        <p className="mb-3 text-sm text-gray-600">
          The AI reads the target role <span className="font-medium text-gray-800">{personaName}</span> (details, skills and experience),
          chooses one of the CV designs and creates the CV. It only uses what is already filled in, and the CV appears in the list below
          when it is done.
        </p>
        <label className="block text-xs font-medium text-gray-500">
          What is this CV for? (optional)
          <textarea
            value={focus}
            onChange={(e) => {
              setFocus(e.target.value)
            }}
            maxLength={500}
            rows={3}
            placeholder="e.g. data roles at startups, or a conservative design for banks"
            className={`mt-1 ${inputClass}`}
          />
        </label>
        {photoLine && (
          <p data-testid="cv-ai-photo-line" className="mt-2 text-xs text-gray-500">
            {photoLine}
          </p>
        )}
        {usable.length > 0 && (
          <>
            <p className="mt-4 text-xs font-medium text-gray-500">AI for this CV</p>
            <CvAiPicker
              choice={choice}
              settings={{ platform: ai.platform, model: ai.model }}
              platforms={usable}
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
          This target role&apos;s data is sent to <span className="font-medium text-gray-700">{aiText || 'the AI platform'}</span> to do
          this.
          {usable.length === 0 && (
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
        {error && <p className="mt-3 text-sm text-red-700">{error}</p>}
        <div className="mt-5 flex justify-end gap-2">
          <button
            type="button"
            onClick={close}
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
    </>
  )
}
