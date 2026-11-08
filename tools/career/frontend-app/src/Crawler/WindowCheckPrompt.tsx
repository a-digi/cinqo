import { useState } from 'react'
import { confirmWindowCleared } from './linkCrawl'

// plan 198: a browser window opened on this computer because the site shows a check only a person can clear. The person clears it there and
// answers here; only then does Katana go on, reading in that same window.
export function WindowCheckPrompt({
  portalLinkId,
  url,
  onCleared,
  onCancel,
  className = '',
}: {
  portalLinkId: string
  url?: string
  onCleared: () => void
  onCancel?: () => void
  className?: string
}) {
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  function yes() {
    setBusy(true)
    setError('')
    confirmWindowCleared(portalLinkId, url)
      .then(() => {
        onCleared()
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : 'Could not pass your answer on.')
      })
      .finally(() => {
        setBusy(false)
      })
  }

  return (
    <div
      role="status"
      data-testid="window-check-prompt"
      className={`rounded-md border border-amber-300 bg-amber-50 px-2 py-1.5 text-xs text-amber-900 ${className}`}
    >
      <p className="font-medium">A browser window opened on this computer. Clear the Cloudflare check there.</p>
      <p className="mt-0.5">Was the check cleared? Then Katana goes on in that same window.</p>
      <div className="mt-1.5 flex gap-2">
        <button
          type="button"
          disabled={busy}
          onClick={yes}
          className="rounded-md bg-amber-700 px-2.5 py-1 text-xs font-medium text-white hover:bg-amber-800 disabled:opacity-50"
        >
          Yes, it was cleared
        </button>
        {onCancel && (
          <button
            type="button"
            disabled={busy}
            onClick={onCancel}
            className="rounded-md border border-amber-300 bg-white px-2.5 py-1 text-xs text-amber-900 hover:bg-amber-100 disabled:opacity-50"
          >
            Not now
          </button>
        )}
      </div>
      {error && (
        <p role="alert" className="mt-1 text-red-700">
          {error}
        </p>
      )}
    </div>
  )
}
