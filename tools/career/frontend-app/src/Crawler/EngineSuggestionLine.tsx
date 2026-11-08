import { useState } from 'react'
import { updatePortalLink, type PortalLink } from '../api'
import { ENGINE_SUGGESTION_NOTE, KEEP_KATANA_LABEL, NOT_NOW_LABEL, USE_ROD_LABEL, engineSuggestionText, offersKatanaWindow, offersRod } from './engineSuggestion'
import { openKatanaWindow } from './linkCrawl'
import { WindowCheckPrompt } from './WindowCheckPrompt'

// EngineSuggestionLine (plan 197, step 4): one orange line on a link whose Katana read the site stopped. "Use Rod for this link" stores the engine on
// the link (it is remembered); "Not now" hides it until a newer block. Shown only while the link follows Katana (the backend sends it only then).
export function EngineSuggestionLine({ link, onReload }: { link: Pick<PortalLink, 'id' | 'engineSuggestion'>; onReload: () => void }) {
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [windowOpened, setWindowOpened] = useState(false)
  const suggestion = link.engineSuggestion
  if (suggestion === null) return null
  const rod = offersRod(suggestion.code)

  function act(args: { crawlEngine?: 'rod'; engineHintDismissed?: boolean }) {
    setBusy(true)
    setError('')
    updatePortalLink(link.id, args)
      .then(onReload)
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : 'Could not save the choice.')
      })
      .finally(() => {
        setBusy(false)
      })
  }

  // plan 198: open the Katana window; the person clears the check there and answers below, then Katana reads in that window
  function keepKatana() {
    setBusy(true)
    setError('')
    openKatanaWindow(link.id)
      .then(() => {
        setWindowOpened(true)
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : 'Could not open the browser window.')
      })
      .finally(() => {
        setBusy(false)
      })
  }

  return (
    <div role="status" className="mb-2 rounded-md border border-amber-300 bg-amber-50 px-2 py-1.5 text-xs text-amber-900">
      <p className="font-medium">{engineSuggestionText(suggestion.code)}</p>
      {rod && <p className="mt-0.5 text-amber-800">{ENGINE_SUGGESTION_NOTE}</p>}
      <div className="mt-1.5 flex gap-2">
        {offersKatanaWindow(suggestion.code) && !windowOpened && (
          <button
            type="button"
            disabled={busy}
            onClick={keepKatana}
            className="rounded-md bg-amber-700 px-2.5 py-1 text-xs font-medium text-white hover:bg-amber-800 disabled:opacity-50"
          >
            {KEEP_KATANA_LABEL}
          </button>
        )}
        {rod && (
          <button
            type="button"
            disabled={busy}
            onClick={() => {
              act({ crawlEngine: 'rod' })
            }}
            className="rounded-md bg-amber-700 px-2.5 py-1 text-xs font-medium text-white hover:bg-amber-800 disabled:opacity-50"
          >
            {USE_ROD_LABEL}
          </button>
        )}
        <button
          type="button"
          disabled={busy}
          onClick={() => {
            act({ engineHintDismissed: true })
          }}
          className="rounded-md border border-amber-300 bg-white px-2.5 py-1 text-xs text-amber-900 hover:bg-amber-100 disabled:opacity-50"
        >
          {rod ? NOT_NOW_LABEL : 'Dismiss'}
        </button>
      </div>
      {windowOpened && (
        <WindowCheckPrompt
          className="mt-1.5"
          portalLinkId={link.id}
          onCleared={() => {
            act({ engineHintDismissed: true }) // the check is cleared: the suggestion has done its job
          }}
          onCancel={() => {
            setWindowOpened(false)
          }}
        />
      )}
      {error && (
        <p role="alert" className="mt-1 text-red-700">
          {error}
        </p>
      )}
    </div>
  )
}
