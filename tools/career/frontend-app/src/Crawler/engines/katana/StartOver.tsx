import { useState } from 'react'
import { deleteConversation } from '../../../Cinqo/Conversation/conversation'
import { Modal } from '../../../Shared/Modal/Modal'
import { runStartFresh, startFreshResultLine } from '../../startFresh'
import type { StartOverProps } from '../types'
import { katanaStartOver, START_OVER_KEEPS, START_OVER_OPTIONS, type StartOverScope } from './startOverApi'

// The Katana engine's "Start over": one button, a confirmation with the two choices, then the server clears what Katana made for this link
// and the screen deletes the AI conversations of the cleared attempts. Crawled jobs and Rod's own instructions stay.
export function StartOver({ link, blockedReason, onReload }: StartOverProps) {
  const [open, setOpen] = useState(false)
  const [scope, setScope] = useState<StartOverScope>('all')
  const [working, setWorking] = useState(false)
  const [error, setError] = useState('')
  const [result, setResult] = useState('')
  const idBase = `start-over-${link.id}`

  function close() {
    if (!working) setOpen(false)
  }

  function confirm() {
    setWorking(true)
    setError('')
    runStartFresh({ startFresh: (id) => katanaStartOver(id, scope), deleteConversation }, link.id, false)
      .then((outcome) => {
        setResult(startFreshResultLine(outcome))
        setOpen(false)
        onReload()
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
      .finally(() => {
        setWorking(false)
      })
  }

  return (
    <div className="mt-2">
      <div className="flex flex-wrap items-center gap-2">
        <button
          type="button"
          disabled={blockedReason !== null}
          title={blockedReason ?? "Delete this link's Katana instructions so the AI can begin again"}
          onClick={() => {
            setScope('all')
            setError('')
            setResult('')
            setOpen(true)
          }}
          className="rounded-full border border-gray-200 bg-white px-2.5 py-0.5 text-xs text-gray-700 hover:bg-gray-50 disabled:cursor-not-allowed disabled:opacity-50"
        >
          Start over
        </button>
        {result !== '' && (
          <p role="status" className="text-xs text-gray-600">
            {result}
          </p>
        )}
      </div>
      <Modal open={open} title="Start over" onClose={close}>
        <fieldset className="mb-3 space-y-2" disabled={working}>
          <legend className="mb-1 text-sm text-gray-600">What should be deleted for this link?</legend>
          {START_OVER_OPTIONS.map((o) => (
            <div key={o.scope} className="flex items-start gap-2">
              <input
                id={`${idBase}-${o.scope}`}
                type="radio"
                name={idBase}
                checked={scope === o.scope}
                onChange={() => {
                  setScope(o.scope)
                }}
                className="mt-0.5"
              />
              <label htmlFor={`${idBase}-${o.scope}`} className="text-sm text-gray-800">
                {o.label}
                <span className="block text-xs text-gray-500">{o.detail}</span>
              </label>
            </div>
          ))}
        </fieldset>
        <p className="mb-3 text-xs text-gray-600">{START_OVER_KEEPS}</p>
        {error !== '' && (
          <p role="alert" className="mb-3 rounded-md border border-red-200 bg-red-50 px-3 py-2 text-xs text-red-800">
            {error}
          </p>
        )}
        <div className="flex justify-end gap-2">
          <button
            type="button"
            onClick={close}
            disabled={working}
            className="rounded-md border border-gray-200 px-3.5 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50 disabled:opacity-50"
          >
            Cancel
          </button>
          <button
            type="button"
            onClick={confirm}
            disabled={working}
            className="rounded-md bg-red-600 px-3.5 py-2 text-sm font-medium text-white hover:bg-red-700 disabled:opacity-50"
          >
            {working ? 'Working…' : 'Start over'}
          </button>
        </div>
      </Modal>
    </div>
  )
}
