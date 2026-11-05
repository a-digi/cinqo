import { useState } from 'react'
import { deleteConversation } from '../../Cinqo/Conversation/conversation'
import { startFreshPortalLink, type PortalLink } from '../../api'
import { Modal } from '../../Shared/Modal/Modal'
import {
  runStartFresh,
  startFreshBlockedReason,
  startFreshExplanation,
  startFreshResultLine,
  START_FRESH_CLEAR_INSTRUCTIONS_LABEL,
  START_FRESH_GENERATE_LABEL,
} from '../startFresh'

// StartFreshButton (step 161) — one link's "Start fresh": a confirmation that deletes the conversations of the
// link's earlier AI attempts (with their prompt and log files), clears the attempt history, and optionally the
// stored instructions, then optionally starts a new generation. See
// plan/ai/tools/career/step-161-start-fresh-for-a-link.md.
export function StartFreshButton({
  link,
  tabBusy,
  canGenerate,
  cannotGenerateReason,
  onGenerate,
  onReload,
}: {
  link: PortalLink
  tabBusy: { listingGenerating: boolean; jobDetailGenerating: boolean; inheriting: boolean }
  // False while no AI platform is configured: the "generate again" choice is then off and disabled.
  canGenerate: boolean
  // Why "generate again" is off when it is not for lack of an AI platform (a link that needs a login: the AI cannot read it).
  cannotGenerateReason?: string
  onGenerate: () => void
  onReload: () => void
}) {
  const [open, setOpen] = useState(false)
  const [clearInstructions, setClearInstructions] = useState(false)
  const [generateAgain, setGenerateAgain] = useState(true)
  const [working, setWorking] = useState(false)
  const [error, setError] = useState('')
  const [result, setResult] = useState('')

  const blocked = startFreshBlockedReason(link, tabBusy)
  const idBase = `start-fresh-${link.id}`

  function close() {
    if (!working) setOpen(false)
  }

  function confirm() {
    setWorking(true)
    setError('')
    runStartFresh({ startFresh: startFreshPortalLink, deleteConversation }, link.id, clearInstructions)
      .then((outcome) => {
        setResult(startFreshResultLine(outcome))
        setOpen(false)
        onReload()
        if (generateAgain && canGenerate) onGenerate()
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
          disabled={blocked !== null}
          title={blocked ?? "Delete this link's earlier AI attempts and conversations and begin again"}
          onClick={() => {
            setClearInstructions(false)
            setGenerateAgain(canGenerate)
            setError('')
            setResult('')
            setOpen(true)
          }}
          className="rounded-full border border-gray-200 bg-white px-2.5 py-0.5 text-xs text-gray-700 hover:bg-gray-50 disabled:cursor-not-allowed disabled:opacity-50"
        >
          Start fresh
        </button>
        {result !== '' && (
          <p role="status" className="text-xs text-gray-600">
            {result}
          </p>
        )}
      </div>
      <Modal open={open} title="Start fresh" onClose={close}>
        <p className="mb-3 text-sm text-gray-600">{startFreshExplanation(link)}</p>
        <div className="mb-3 space-y-2">
          <div className="flex items-start gap-2">
            <input
              id={`${idBase}-clear`}
              type="checkbox"
              checked={clearInstructions}
              disabled={working}
              onChange={(e) => {
                setClearInstructions(e.target.checked)
              }}
              className="mt-0.5"
            />
            <label htmlFor={`${idBase}-clear`} className="text-sm text-gray-800">
              {START_FRESH_CLEAR_INSTRUCTIONS_LABEL}
            </label>
          </div>
          <div className="flex items-start gap-2">
            <input
              id={`${idBase}-generate`}
              type="checkbox"
              checked={generateAgain && canGenerate}
              disabled={working || !canGenerate}
              onChange={(e) => {
                setGenerateAgain(e.target.checked)
              }}
              className="mt-0.5"
            />
            <label htmlFor={`${idBase}-generate`} className="text-sm text-gray-800">
              {START_FRESH_GENERATE_LABEL}
              {!canGenerate && (
                <span className="block text-xs text-gray-500">{cannotGenerateReason ?? 'No AI platform is configured.'}</span>
              )}
            </label>
          </div>
        </div>
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
            {working ? 'Working…' : 'Start fresh'}
          </button>
        </div>
      </Modal>
    </div>
  )
}
