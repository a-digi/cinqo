import { useCallback, useEffect, useState } from 'react'
import { fetchKatanaWindowSites, forgetKatanaWindowSite, type KatanaWindowSite } from './api'
import { cardClassName, cardStyle } from './Shared/cardStyle'

function formatTime(iso: string | undefined): string {
  if (!iso) return ''
  const d = new Date(iso.includes('T') ? iso : `${iso.replace(' ', 'T')}Z`)
  return Number.isNaN(d.getTime()) ? iso : d.toLocaleString()
}

// Katana window sites (plan 198, step V3): the sites whose Cloudflare check you cleared in the Katana window. Katana reads them in that window
// (while it is open). "Forget" sends a site back to the headless way and removes its cookies from the window's profile.
export function KatanaWindowSites() {
  const [sites, setSites] = useState<KatanaWindowSite[] | null>(null)
  const [windowOpen, setWindowOpen] = useState(false)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState('')

  const load = useCallback(() => {
    fetchKatanaWindowSites()
      .then((r) => {
        setSites(r.hosts)
        setWindowOpen(r.windowOpen)
        setError('')
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : 'Could not load the window sites.')
      })
  }, [])

  useEffect(() => {
    load()
  }, [load])

  function forget(host: string) {
    setBusy(host)
    forgetKatanaWindowSite(host)
      .then(load)
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : 'Could not forget the site.')
      })
      .finally(() => {
        setBusy('')
      })
  }

  return (
    <section className={`${cardClassName} mb-5`} style={cardStyle} data-testid="katana-window-sites">
      <h2 className="mb-1 text-base font-semibold text-gray-900">Sites read in the Katana window</h2>
      <p className="mb-3 text-sm text-gray-500">
        Sites whose Cloudflare check you cleared in the Katana window. While that window is open ({windowOpen ? 'it is open now' : 'it is closed now'}),
        Katana reads these sites in it. Forget a site to read it the headless way again; its cookies are removed from the window&apos;s profile.
      </p>
      {error && <p className="mb-2 text-sm text-red-700">{error}</p>}
      {sites !== null && sites.length === 0 && <p className="text-sm text-gray-500">No site yet.</p>}
      <ul className="space-y-2">
        {(sites ?? []).map((s) => (
          <li key={s.host} className="flex flex-wrap items-center gap-3 text-sm">
            <span className="font-medium text-gray-900">{s.host}</span>
            <span className="text-gray-500">cleared {formatTime(s.lastClearedAt ?? s.createdAt)}</span>
            <button
              type="button"
              disabled={busy === s.host}
              onClick={() => {
                forget(s.host)
              }}
              className="rounded-md border border-red-200 bg-white px-2 py-0.5 text-xs text-red-700 hover:bg-red-50 disabled:opacity-50"
            >
              Forget
            </button>
          </li>
        ))}
      </ul>
    </section>
  )
}
