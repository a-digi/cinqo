import { useEffect, useState } from 'react'
import {
  fetchBrowserSettings,
  fetchEngineStatus,
  saveBrowserSettings,
  testPlaywrightEngine,
  type BrowserSettings,
  type EngineStatus,
  type EngineTestResult,
  type EngineStatusResponse,
  type FetchEngine,
} from './api'

// Engine settings page (step 50, availability added in step 51) — one
// checkbox choosing which engine fetches pages. Checked means Rod (the
// default), unchecked means Playwright. Stored in the same settings object
// as the Debug toggles and read by the backend on every fetch, so the
// choice applies to the next page fetched, with no restart. The whole object
// is saved on every change (the backend overwrites it on a PUT), with the
// Debug values exactly as they were loaded.
//
// Each engine's availability comes from GET /engine-status. Playwright does
// NOT use the Node installed in the OS: it needs its own driver (which
// bundles a Node) installed beforehand, so when that is missing the box is
// locked on Rod — unchecking it would only make every fetch fall back to the
// built-in browser path. The server still accepts either value; this gating
// lives in the page. See
// plan/ai/tools/browser/step-50-fetch-engine-setting-page.md and
// step-51-engine-availability.md.
export function EnginePage() {
  const [settings, setSettings] = useState<BrowserSettings | null>(null)
  const [status, setStatus] = useState<EngineStatusResponse | null>(null)
  const [statusError, setStatusError] = useState('')
  const [error, setError] = useState('')
  const [saving, setSaving] = useState(false)
  // "Test Playwright" (step 52): a real launch of its Chromium, so a driver
  // installed without its browser is caught. The result stays on screen
  // until the next test.
  const [testing, setTesting] = useState(false)
  const [testResult, setTestResult] = useState<EngineTestResult | null>(null)
  const [testError, setTestError] = useState('')

  useEffect(() => {
    fetchBrowserSettings()
      .then(setSettings)
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
    // The status is advice, never a precondition: if it cannot be read the
    // page still works and the box behaves as it did before availability
    // existed.
    fetchEngineStatus()
      .then(setStatus)
      .catch((err: unknown) => {
        setStatusError(err instanceof Error ? err.message : String(err))
      })
  }, [])

  function runPlaywrightTest() {
    setTesting(true)
    setTestResult(null)
    setTestError('')
    testPlaywrightEngine()
      .then(setTestResult)
      .catch((err: unknown) => {
        setTestError(err instanceof Error ? err.message : String(err))
      })
      .finally(() => {
        setTesting(false)
      })
  }

  function update(engine: FetchEngine) {
    save({ fetchEngine: engine })
  }

  // save applies a change on top of the loaded settings and sends the whole
  // object (the backend overwrites it on a PUT).
  function save(change: Partial<BrowserSettings>) {
    if (!settings) return
    const previous = settings
    setError('')
    setSaving(true)
    setSettings({ ...settings, ...change }) // optimistic — reverted below if the save fails
    saveBrowserSettings({ ...settings, ...change })
      .then(setSettings)
      .catch((err: unknown) => {
        setSettings(previous)
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

  // A backend that predates the setting sends no fetchEngine: that is Rod,
  // the default.
  const useRod = settings.fetchEngine !== 'playwright'
  const humanAssist = settings.humanAssist
  const playwrightUnavailable = status !== null && !status.playwright.available
  const rodUnavailable = status !== null && !status.rod.available
  // Unchecking would select Playwright: only offered when it can run. A
  // stored Playwright choice is always re-checkable (switching back to Rod
  // must never be blocked).
  const lockedOnRod = useRod && playwrightUnavailable

  return (
    <div className="max-w-2xl p-6 font-sans text-gray-900">
      <h1 className="mb-1.5 text-xl font-semibold">Engine</h1>
      <p className="mb-5 text-sm text-gray-500">
        Chooses which engine fetches a page the first time. Rod is the default. Both need Chrome installed, and if the chosen engine
        can&apos;t start, the fetch falls back to the built-in browser path automatically.
      </p>

      <div className="min-h-[1.2em] text-sm text-red-700">{error}</div>

      {!useRod && playwrightUnavailable && (
        <div className="mb-3 rounded-md border border-amber-300 bg-amber-50 p-3 text-sm text-amber-900">
          Playwright is selected but it is not available, so every fetch is currently falling back to the built-in browser path.{' '}
          {status.playwright.detail}. Switch back to Rod, or install the Playwright driver.
        </div>
      )}
      {useRod && rodUnavailable && (
        <div className="mb-3 rounded-md border border-amber-300 bg-amber-50 p-3 text-sm text-amber-900">
          Rod is selected but it cannot start, so every fetch is currently falling back to the built-in browser path. {status.rod.detail}.
        </div>
      )}

      <div className="space-y-4 rounded-md border border-gray-200 bg-gray-50 p-6 shadow-sm">
        {/* aria-label given explicitly — the visible label text is nested
            two <span> levels deep, past what jsx-a11y's own accessible-text
            detection reliably walks. */}
        <label
          htmlFor="cinqo-browser-engine-rod"
          aria-label="Use Rod"
          className={`flex items-start gap-3 ${lockedOnRod ? 'opacity-70' : ''}`}
        >
          <input
            id="cinqo-browser-engine-rod"
            type="checkbox"
            checked={useRod}
            disabled={saving || lockedOnRod}
            onChange={(e) => {
              update(e.target.checked ? 'rod' : 'playwright')
            }}
            className="mt-0.5 h-4 w-4 rounded border-gray-300"
          />
          <span>
            <span className="block text-sm font-medium">Use Rod (recommended)</span>
            <span className="block text-xs text-gray-500">
              Checked: pages are fetched with Rod — pure Go over the Chrome DevTools Protocol, needs nothing but Chrome. Unchecked:
              Playwright — it needs its own driver (which brings its own Node) installed first; it does not use the Node installed on your
              computer, and nothing is downloaded when it is first used.
            </span>
          </span>
        </label>

        {lockedOnRod && (
          <p className="text-xs text-gray-600">
            Playwright can&apos;t be chosen here until its driver is installed: {status.playwright.detail}.
          </p>
        )}

        <p className="text-xs text-gray-500">
          Currently using: <span className="font-medium text-gray-700">{useRod ? 'Rod' : 'Playwright'}</span>. A change applies to the next
          page fetched — no restart needed, and a fetch already running is not interrupted.
        </p>
      </div>

      <h2 className="mb-1.5 mt-8 text-lg font-semibold">When a site asks for a check</h2>
      <div className="space-y-2 rounded-md border border-gray-200 bg-gray-50 p-6 shadow-sm">
        <label htmlFor="cinqo-browser-human-assist" aria-label="Ask me to clear it" className="flex items-start gap-3">
          <input
            id="cinqo-browser-human-assist"
            type="checkbox"
            checked={humanAssist}
            disabled={saving}
            onChange={(e) => {
              save({ humanAssist: e.target.checked })
            }}
            className="mt-0.5 h-4 w-4 rounded border-gray-300"
          />
          <span>
            <span className="block text-sm font-medium">Open a browser window for me to clear it (recommended)</span>
            <span className="block text-xs text-gray-500">
              When an AI agent reaches a site that asks for a check only a person can pass, a Chrome window opens on this computer so you
              can clear it. The agent is told right away that you were asked, so it does not wait. Once you have cleared it, the clearance
              is kept until it expires and the agent&apos;s next call works. The tool never solves the check itself. Unchecked: the agent
              just gets told that the site needs a person.
            </span>
          </span>
        </label>
      </div>

      <h2 className="mb-1.5 mt-8 text-lg font-semibold">Availability</h2>
      <p className="mb-3 text-sm text-gray-500">What each engine needs, and whether it is there right now.</p>
      {statusError && <div className="mb-3 text-sm text-red-700">{statusError}</div>}
      {status ? (
        <div className="space-y-2">
          <AvailabilityRow name="Rod" status={status.rod} />
          <AvailabilityRow name="Playwright" status={status.playwright} />
        </div>
      ) : (
        !statusError && <p className="text-sm text-gray-400">Checking…</p>
      )}
      <p className="mt-3 text-xs text-gray-400">
        The availability above does not launch a browser, so it cannot tell whether Playwright&apos;s own Chromium is installed. The test
        below does.
      </p>

      <div className="mt-4 flex flex-wrap items-center gap-3">
        <button
          type="button"
          onClick={runPlaywrightTest}
          disabled={testing}
          className="rounded-md border border-gray-300 bg-white px-3 py-1.5 text-sm font-medium text-gray-800 hover:bg-gray-50 disabled:cursor-not-allowed disabled:opacity-50"
        >
          {testing ? 'Testing Playwright…' : 'Test Playwright'}
        </button>
        {testing && <span className="text-xs text-gray-500">Launching Chromium — this can take a few seconds.</span>}
        {testResult && (
          <span role="status" className={`text-sm ${testResult.ok ? 'text-green-700' : 'text-red-700'}`}>
            {testResult.ok ? '✓' : '✗'} {testResult.detail}
            {testResult.ok && ` (${(testResult.durationMs / 1000).toFixed(1)} s)`}
          </span>
        )}
        {testError && <span className="text-sm text-red-700">{testError}</span>}
      </div>
    </div>
  )
}

function AvailabilityRow({ name, status }: { name: string; status: EngineStatus }) {
  return (
    <div className="flex items-start gap-3 rounded-md border border-gray-200 bg-white p-3">
      <span
        className={`mt-0.5 shrink-0 rounded-full px-2 py-0.5 text-xs font-medium ${
          status.available ? 'bg-green-100 text-green-800' : 'bg-red-100 text-red-800'
        }`}
      >
        {status.available ? 'Available' : 'Not available'}
      </span>
      <span className="min-w-0">
        <span className="block text-sm font-medium">{name}</span>
        <span className="block break-words text-xs text-gray-500">{status.detail}</span>
      </span>
    </div>
  )
}
