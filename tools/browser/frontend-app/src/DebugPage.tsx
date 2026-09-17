import { useEffect, useState } from 'react'
import { fetchBrowserSettings, saveBrowserSettings, fetchCloudflareDomains, type BrowserSettings, type CloudflareDomain } from './api'

// Debug settings page (step 22) — a single object with two booleans.
// Debug active gates ALL crawl logging: off (the confirmed default),
// crawl_paginated writes nothing to crawl_logs at all, not merely a
// hidden entry. Log HTML additionally captures each logged page's own
// raw rendered HTML — meaningful only when Debug is also on, so its
// own toggle is disabled (not just inert) whenever Debug is off. See
// plan/ai/tools/browser/step-22-debug-mode-and-log-management.md.
export function DebugPage() {
  const [settings, setSettings] = useState<BrowserSettings | null>(null)
  const [error, setError] = useState('')
  const [saving, setSaving] = useState(false)

  // Cloudflare domain cache (step 30) — read-only; step 29's own cache
  // has no manual removal path yet (see that step's own Open Question
  // 1), so this is purely observability for now.
  const [cloudflareDomains, setCloudflareDomains] = useState<CloudflareDomain[] | null>(null)
  const [cloudflareDomainsError, setCloudflareDomainsError] = useState('')

  useEffect(() => {
    fetchBrowserSettings()
      .then(setSettings)
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
    fetchCloudflareDomains()
      .then(setCloudflareDomains)
      .catch((err: unknown) => {
        setCloudflareDomainsError(err instanceof Error ? err.message : String(err))
      })
  }, [])

  function update(next: BrowserSettings) {
    setError('')
    setSaving(true)
    setSettings(next) // optimistic — reverted below if the save fails
    saveBrowserSettings(next)
      .then(setSettings)
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
      .finally(() => {
        setSaving(false)
      })
  }

  if (!settings) {
    return (
      <div className="max-w-2xl p-6 font-sans text-gray-900">
        <div className="min-h-[1.2em] text-sm text-red-700">{error}</div>
      </div>
    )
  }

  return (
    <div className="max-w-2xl p-6 font-sans text-gray-900">
      <h1 className="mb-1.5 text-xl font-semibold">Debug</h1>
      <p className="mb-5 text-sm text-gray-500">
        Controls whether the Browser tool records anything to Crawl Logs at all. Off by default — turn Debug on to start logging
        crawl_paginated calls again.
      </p>

      <div className="min-h-[1.2em] text-sm text-red-700">{error}</div>

      <div className="space-y-4 rounded-md border border-gray-200 bg-gray-50 p-6 shadow-sm">
        {/* aria-label given explicitly — the visible label text is nested
            two <span> levels deep, past what jsx-a11y's own accessible-text
            detection reliably walks. */}
        <label htmlFor="cinqo-browser-debug-enabled" aria-label="Debug active" className="flex items-start gap-3">
          <input
            id="cinqo-browser-debug-enabled"
            type="checkbox"
            checked={settings.debugEnabled}
            disabled={saving}
            onChange={(e) => {
              update({ ...settings, debugEnabled: e.target.checked })
            }}
            className="mt-0.5 h-4 w-4 rounded border-gray-300"
          />
          <span>
            <span className="block text-sm font-medium">Debug active</span>
            <span className="block text-xs text-gray-500">
              When off, no crawl is ever logged — nothing is written to Crawl Logs, not just hidden from view.
            </span>
          </span>
        </label>

        <label
          htmlFor="cinqo-browser-debug-log-html"
          aria-label="Log HTML"
          className={`flex items-start gap-3 ${settings.debugEnabled ? '' : 'opacity-50'}`}
        >
          <input
            id="cinqo-browser-debug-log-html"
            type="checkbox"
            checked={settings.debugLogHtml}
            disabled={saving || !settings.debugEnabled}
            onChange={(e) => {
              update({ ...settings, debugLogHtml: e.target.checked })
            }}
            className="mt-0.5 h-4 w-4 rounded border-gray-300"
          />
          <span>
            <span className="block text-sm font-medium">Log HTML</span>
            <span className="block text-xs text-gray-500">
              Only takes effect while Debug is active. Also captures each logged page's own raw rendered HTML — useful for understanding
              what a crawl actually saw (e.g. a Cloudflare challenge), but meaningfully larger and slower to log than results alone.
            </span>
          </span>
        </label>

        <label
          htmlFor="cinqo-browser-debug-log-challenge"
          aria-label="Challenge logs"
          className={`flex items-start gap-3 ${settings.debugEnabled ? '' : 'opacity-50'}`}
        >
          <input
            id="cinqo-browser-debug-log-challenge"
            type="checkbox"
            checked={settings.debugLogChallenge}
            disabled={saving || !settings.debugEnabled}
            onChange={(e) => {
              update({ ...settings, debugLogChallenge: e.target.checked })
            }}
            className="mt-0.5 h-4 w-4 rounded border-gray-300"
          />
          <span>
            <span className="block text-sm font-medium">Challenge logs</span>
            <span className="block text-xs text-gray-500">
              Only takes effect while Debug is active. Independent of Log HTML — captures the raw HTML analyzed on each retry while waiting
              for a Cloudflare challenge to be solved, useful specifically for debugging why a challenge wasn't recognized as solved.
            </span>
          </span>
        </label>
      </div>

      <h2 className="mb-1.5 mt-8 text-lg font-semibold">Known Cloudflare domains</h2>
      <p className="mb-3 text-sm text-gray-500">
        Every domain that has ever given the headless session an unresolved Cloudflare challenge — crawls against these now skip straight to
        the headed (non-headless) fallback instead of trying headless first. Read-only; there is currently no way to remove a domain from
        this list once it's been recorded.
      </p>

      {cloudflareDomainsError && <div className="mb-3 text-sm text-red-700">{cloudflareDomainsError}</div>}

      {cloudflareDomains?.length === 0 && <p className="text-sm text-gray-400">No domains recorded yet.</p>}

      {cloudflareDomains && cloudflareDomains.length > 0 && (
        <div className="overflow-x-auto rounded-md border border-gray-200 bg-gray-50 shadow-sm">
          <table className="w-full text-left text-sm">
            <thead>
              <tr className="border-b border-gray-200 text-xs uppercase text-gray-500">
                <th className="px-4 py-2 font-medium">Domain</th>
                <th className="px-4 py-2 font-medium">Reason</th>
                <th className="px-4 py-2 font-medium">First detected</th>
              </tr>
            </thead>
            <tbody>
              {cloudflareDomains.map((d) => (
                <tr key={d.domain} className="border-b border-gray-100 last:border-0">
                  <td className="px-4 py-2 font-mono text-xs">{d.domain}</td>
                  <td className="px-4 py-2 text-xs text-gray-600">{d.reason}</td>
                  <td className="px-4 py-2 text-xs text-gray-500">{new Date(d.firstDetectedAt).toLocaleString()}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}
