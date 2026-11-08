import { useState } from 'react'
import type { AiChoice } from '../Cinqo/Platform/aiChoice'
import { SparkleIcon } from '../Shared/Icons/icons'
import { inputClass } from '../Shared/inputClass'
import { Modal } from '../Shared/Modal/Modal'
import { SETTINGS_PATH } from '../paths'

// The "Create with AI" button and its dialog (step 112): the optional focus, the AI that will be
// used, and one line about the data going to it. The button says why it is disabled.
export function CreateWithAi({
  disabledReason,
  ai,
  onStart,
}: {
  // Empty when the button can be used.
  disabledReason: string
  ai: AiChoice
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
        title={disabledReason || 'Let the AI propose new personas from what you have filled in'}
        onClick={() => {
          setOpen(true)
        }}
        className="flex shrink-0 items-center gap-1.5 rounded-md border border-purple-200 bg-white px-3.5 py-2 text-sm font-medium text-purple-700 transition-colors hover:bg-purple-50 disabled:cursor-not-allowed disabled:opacity-50"
      >
        <SparkleIcon />
        Create with AI
      </button>

      <Modal open={open} title="Create personas with AI" onClose={close}>
        <p className="mb-3 text-sm text-gray-600">
          The AI reads this profile&apos;s personas (details, skills and experience) and proposes up to three new ones. Nothing is saved
          until you have reviewed them.
        </p>
        <label className="block text-xs font-medium text-gray-500">
          What should the personas focus on? (optional)
          <textarea
            value={focus}
            onChange={(e) => {
              setFocus(e.target.value)
            }}
            maxLength={500}
            rows={3}
            placeholder="e.g. one for backend roles, one for management"
            className={`mt-1 ${inputClass}`}
          />
        </label>
        <p className="mt-3 text-xs text-gray-500">
          Your career data is sent to <span className="font-medium text-gray-700">{aiText || 'the AI platform'}</span> to do this.{' '}
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
