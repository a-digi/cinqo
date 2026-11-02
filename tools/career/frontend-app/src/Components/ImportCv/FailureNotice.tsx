// FailureNotice.tsx — how a failed CV analysis is shown (plan 165, step 4): one plain sentence (for a model that is not up to the
// task: say so and point to the AI setting), and a "Show log" button that opens the real error text. The raw text of the core's loop
// or of a provider is for the log, never the first thing a person reads.
import { useState } from 'react'
import { SETTINGS_PATH } from '../../paths'
import { describeFailure } from './runStatusRules'

interface FailureNoticeProps {
  error: string | undefined | null
  // Opens the AI conversation of the run, when there is one.
  onViewTranscript?: () => void
}

export function FailureNotice({ error, onViewTranscript }: FailureNoticeProps) {
  const [logOpen, setLogOpen] = useState(false)
  const failure = describeFailure(error)
  return (
    <div className="text-sm">
      <p className="text-red-700">{failure.message}</p>
      <div className="mt-1 flex flex-wrap items-center gap-3 text-xs">
        {failure.modelNotCapable && (
          <a
            href={SETTINGS_PATH}
            onClick={(e) => {
              e.preventDefault()
              window.__cinqoToolBridge.navigate(SETTINGS_PATH)
            }}
            className="text-blue-700 underline hover:text-blue-800"
          >
            Open AI settings
          </a>
        )}
        {failure.log !== '' && (
          <button
            type="button"
            onClick={() => {
              setLogOpen((open) => !open)
            }}
            aria-expanded={logOpen}
            className="text-gray-500 underline hover:text-gray-700"
          >
            {logOpen ? 'Hide log' : 'Show log'}
          </button>
        )}
        {onViewTranscript && (
          <button type="button" onClick={onViewTranscript} className="text-gray-500 underline hover:text-gray-700">
            View transcript
          </button>
        )}
      </div>
      {logOpen && failure.log !== '' && (
        <pre className="mt-2 max-h-64 overflow-auto whitespace-pre-wrap break-words rounded-md bg-gray-50 p-2 text-xs text-gray-700">
          {failure.log}
        </pre>
      )}
    </div>
  )
}
