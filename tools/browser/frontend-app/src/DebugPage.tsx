import { useEffect, useState } from 'react'
import { fetchBrowserSettings, saveBrowserSettings, type BrowserSettings } from './api'

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

  useEffect(() => {
    fetchBrowserSettings()
      .then(setSettings)
      .catch((err: Error) => setError(err.message))
  }, [])

  function update(next: BrowserSettings) {
    setError('')
    setSaving(true)
    setSettings(next) // optimistic — reverted below if the save fails
    saveBrowserSettings(next)
      .then(setSettings)
      .catch((err: Error) => setError(err.message))
      .finally(() => setSaving(false))
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
        Controls whether the Browser tool records anything to Crawl Logs at all. Off by default —
        turn Debug on to start logging crawl_paginated calls again.
      </p>

      <div className="min-h-[1.2em] text-sm text-red-700">{error}</div>

      <div className="space-y-4 rounded-md border border-gray-200 bg-gray-50 p-6 shadow-sm">
        <label className="flex items-start gap-3">
          <input
            type="checkbox"
            checked={settings.debugEnabled}
            disabled={saving}
            onChange={(e) => update({ ...settings, debugEnabled: e.target.checked })}
            className="mt-0.5 h-4 w-4 rounded border-gray-300"
          />
          <span>
            <span className="block text-sm font-medium">Debug active</span>
            <span className="block text-xs text-gray-500">
              When off, no crawl is ever logged — nothing is written to Crawl Logs, not just
              hidden from view.
            </span>
          </span>
        </label>

        <label className={`flex items-start gap-3 ${settings.debugEnabled ? '' : 'opacity-50'}`}>
          <input
            type="checkbox"
            checked={settings.debugLogHtml}
            disabled={saving || !settings.debugEnabled}
            onChange={(e) => update({ ...settings, debugLogHtml: e.target.checked })}
            className="mt-0.5 h-4 w-4 rounded border-gray-300"
          />
          <span>
            <span className="block text-sm font-medium">Log HTML</span>
            <span className="block text-xs text-gray-500">
              Only takes effect while Debug is active. Also captures each logged page's own raw
              rendered HTML — useful for understanding what a crawl actually saw (e.g. a
              Cloudflare challenge), but meaningfully larger and slower to log than results alone.
            </span>
          </span>
        </label>
      </div>
    </div>
  )
}
