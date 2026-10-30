import { useEffect, useState } from 'react'
import { fetchBrowserSettings, saveBrowserSettings, fetchCloudflareDomains, type BrowserSettings, type CloudflareDomain } from './api'
import { loggingLayers, loggingLevel } from './debugRules'
import { BugIcon } from './icons'
import { LoggingGauge } from './LoggingGauge'
import { PageHeaderCard } from './Shared/PageHeaderCard'
import { Switch } from './Shared/Switch'
import { cardClassName, cardStyle } from './Shared/cardStyle'
import { plural } from './Shared/plural'
import { PAGE_ACCENT } from './Shared/theme'

// Debug settings page (step 22) — a single object with two booleans.
// Debug active gates ALL crawl logging: off (the confirmed default),
// crawl_paginated writes nothing to crawl_logs at all, not merely a
// hidden entry. Log HTML additionally captures each logged page's own
// raw rendered HTML — meaningful only when Debug is also on, so its
// own toggle is disabled (not just inert) whenever Debug is off. A gauge
// on the left shows what is being logged right now. See
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
    const previous = settings
    setError('')
    setSaving(true)
    setSettings(next) // optimistic — reverted below if the save fails
    saveBrowserSettings(next)
      .then(setSettings)
      .catch((err: unknown) => {
        setSettings(previous)
        setError(err instanceof Error ? err.message : String(err))
      })
      .finally(() => {
        setSaving(false)
      })
  }

  const header = (
    <PageHeaderCard
      icon={<BugIcon />}
      title="Debug"
      description="Controls whether the Browser tool records anything to Crawl Logs at all. Off by default — turn Debug on to start logging crawl_paginated calls again."
      pill={settings ? { text: settings.debugEnabled ? 'Debug on' : 'Debug off' } : undefined}
    />
  )

  if (!settings) {
    return (
      <div className="max-w-5xl p-6 font-sans text-gray-900">
        {header}
        <div className="min-h-[1.2em] text-sm text-red-700">{error}</div>
      </div>
    )
  }

  return (
    <div className="max-w-5xl p-6 font-sans text-gray-900">
      {header}

      <div className="flex flex-col gap-5 lg:flex-row lg:items-start">
        <aside className="shrink-0 lg:sticky lg:top-6 lg:w-56">
          <LoggingGauge layers={loggingLayers(settings)} level={loggingLevel(settings)} />
        </aside>

        <div className="min-w-0 flex-1 space-y-4">
          <div className="min-h-[1.2em] text-sm text-red-700">{error}</div>

          <section className={cardClassName} style={cardStyle}>
            <Switch
              checked={settings.debugEnabled}
              disabled={saving}
              label="Debug active"
              onChange={(on) => {
                update({ ...settings, debugEnabled: on })
              }}
            />
            <p className="mt-1.5 text-xs text-gray-500">
              When off, no crawl is ever logged — nothing is written to Crawl Logs, not just hidden from view.
            </p>
          </section>

          <section className={`${cardClassName} ${settings.debugEnabled ? '' : 'opacity-60'}`} style={cardStyle}>
            <Switch
              checked={settings.debugLogHtml}
              disabled={saving || !settings.debugEnabled}
              label="Log HTML"
              onChange={(on) => {
                update({ ...settings, debugLogHtml: on })
              }}
            />
            <p className="mt-1.5 text-xs text-gray-500">
              Only takes effect while Debug is active. Also captures each logged page&apos;s own raw rendered HTML — useful for
              understanding what a crawl actually saw (e.g. a Cloudflare challenge), but meaningfully larger and slower to log than results
              alone.
            </p>
          </section>

          <section className={`${cardClassName} ${settings.debugEnabled ? '' : 'opacity-60'}`} style={cardStyle}>
            <Switch
              checked={settings.debugLogChallenge}
              disabled={saving || !settings.debugEnabled}
              label="Challenge logs"
              onChange={(on) => {
                update({ ...settings, debugLogChallenge: on })
              }}
            />
            <p className="mt-1.5 text-xs text-gray-500">
              Only takes effect while Debug is active. Independent of Log HTML — captures the raw HTML analyzed on each retry while waiting
              for a Cloudflare challenge to be solved, useful specifically for debugging why a challenge wasn&apos;t recognized as solved.
            </p>
          </section>

          <section className={cardClassName} style={cardStyle}>
            <div className="mb-1 flex items-center justify-between gap-2">
              <h2 className="text-base font-semibold text-gray-900">Known Cloudflare domains</h2>
              {cloudflareDomains && (
                <span
                  className="rounded-full px-2.5 py-0.5 text-xs font-semibold"
                  style={{ background: PAGE_ACCENT.soft, color: PAGE_ACCENT.text }}
                >
                  {plural(cloudflareDomains.length, 'domain', 'domains')}
                </span>
              )}
            </div>
            <p className="mb-3 text-sm text-gray-500">
              Every domain that has ever given the headless session an unresolved Cloudflare challenge — crawls against these now skip
              straight to the headed (non-headless) fallback instead of trying headless first. Read-only; there is currently no way to
              remove a domain from this list once it&apos;s been recorded.
            </p>

            {cloudflareDomainsError && <div className="mb-3 text-sm text-red-700">{cloudflareDomainsError}</div>}

            {cloudflareDomains?.length === 0 && <p className="text-sm text-gray-400">No domains recorded yet.</p>}

            {cloudflareDomains && cloudflareDomains.length > 0 && (
              <ul className="space-y-2">
                {cloudflareDomains.map((d) => (
                  <li key={d.domain} className="rounded-2xl border border-gray-200 bg-white px-4 py-2.5">
                    <div className="break-all font-mono text-xs font-medium text-gray-900">{d.domain}</div>
                    <div className="text-xs text-gray-600">{d.reason}</div>
                    <div className="text-xs text-gray-400">First detected {new Date(d.firstDetectedAt).toLocaleString()}</div>
                  </li>
                ))}
              </ul>
            )}
          </section>
        </div>
      </div>
    </div>
  )
}
