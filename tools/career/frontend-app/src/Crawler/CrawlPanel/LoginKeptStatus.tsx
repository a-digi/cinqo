import { useEffect, useState } from 'react'
import { FORGET_LOGIN_LABEL, fetchLinkLoginSession, forgetLinkLoginSession, keptLoginText, type LoginSessionInfo } from '../loginSession'

// LoginKeptStatus (plan 195, step 2): shows the login kept for a link that needs a login, with "Forget this login". Shows nothing when the
// link has none. `refreshKey` changes when a login was just kept, so the line appears without a page reload.
export function LoginKeptStatus({ linkId, refreshKey }: { linkId: string; refreshKey: number }) {
  const [info, setInfo] = useState<LoginSessionInfo | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    let cancelled = false
    fetchLinkLoginSession(linkId)
      .then((got) => {
        if (!cancelled) setInfo(got)
      })
      .catch(() => {
        if (!cancelled) setInfo(null)
      })
    return () => {
      cancelled = true
    }
  }, [linkId, refreshKey])

  if (info === null) return null

  function forget() {
    setBusy(true)
    setError('')
    forgetLinkLoginSession(linkId)
      .then(() => {
        setInfo(null)
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : 'Could not forget the login.')
      })
      .finally(() => {
        setBusy(false)
      })
  }

  return (
    <div role="status" className="mt-1.5 flex flex-wrap items-center gap-2 text-xs text-gray-700">
      <span>{keptLoginText(info)}</span>
      <button
        type="button"
        disabled={busy}
        onClick={forget}
        className="rounded-md border border-gray-300 bg-white px-2 py-0.5 text-xs text-gray-700 hover:bg-gray-50 disabled:opacity-50"
      >
        {FORGET_LOGIN_LABEL}
      </button>
      {error && (
        <span role="alert" className="text-red-700">
          {error}
        </span>
      )}
    </div>
  )
}
