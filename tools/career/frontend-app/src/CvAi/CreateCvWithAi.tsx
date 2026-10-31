import { useState } from 'react'
import type { AiChoice } from '../Cinqo/Platform/aiChoice'
import { SparkleIcon } from '../Shared/Icons/icons'
import { inputClass } from '../Shared/inputClass'
import { Modal } from '../Shared/Modal/Modal'
import { SETTINGS_PATH } from '../paths'

// The "Create CV with AI" button and its dialog (step 115): the optional note on what the CV is for,
// the AI that will be used, and one line about the data going to it. The button says why it is
// disabled. The manual "New CV" next to it is untouched.
export function CreateCvWithAi({
  disabledReason,
  ai,
  personaName,
  onStart,
}: {
  // Empty when the button can be used.
  disabledReason: string
  ai: AiChoice
  personaName: string
  onStart: (focus: string) => Promise<void>
}) {
  const [open, setOpen] = useState(false)
  const [focus, setFocus] = useState('')
  const [starting, setStarting] = useState(false)
  const [error, setError] = useState('')

  function close() {
    if (starting) return
    setOpen(false)
    setError('')
  }

  function start() {
    setError('')
    setStarting(true)
    onStart(focus)
      .then(() => {
        setOpen(false)
        setFocus('')
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
      .finally(() => {
        setStarting(false)
      })
  }

  const aiText = ai.platform ? `${ai.platform.name}${ai.model ? ` · ${ai.model}` : ''}` : ''

  return (
    <>
      <button
        type="button"
        disabled={disabledReason !== ''}
        title={disabledReason || 'Let the AI choose a design and create a CV from this persona'}
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
          The AI reads the persona <span className="font-medium text-gray-800">{personaName}</span> (details, skills and experience),
          chooses one of the CV designs and creates the CV. It only uses what is already filled in, and the CV appears in the list below
          when it is done. It has no photo; you can make a CV by hand if you want one.
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
        <p className="mt-3 text-xs text-gray-500">
          This persona&apos;s data is sent to <span className="font-medium text-gray-700">{aiText || 'the AI platform'}</span> to do this.{' '}
          <button
            type="button"
            onClick={() => {
              window.__cinqoToolBridge.navigate(SETTINGS_PATH)
            }}
            className="text-blue-700 underline"
          >
            Change it in Settings
          </button>
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
            disabled={starting}
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
