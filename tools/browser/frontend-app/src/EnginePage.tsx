import type { useEngine } from './useBrowserStatus'
import { cardClassName, cardStyle } from './Shared/cardStyle'
import { Switch } from './Shared/Switch'

// Engine section of the "Browser & Engine" page (step 50, availability added in step 51) — one
// switch choosing which engine fetches pages. On means Rod (the default), off means
// Playwright. Stored in the same settings object as the Debug toggles and read by the
// backend on every fetch, so the choice applies to the next page fetched, with no
// restart. The whole object is saved on every change (the backend overwrites it on a
// PUT), with the Debug values exactly as they were loaded (see useEngine).
//
// Each engine's availability comes from GET /engine-status and is shown in the health
// strip above. Playwright does NOT use the Node installed in the OS: it needs its own
// driver (which bundles a Node) installed beforehand, so when that is missing the switch
// is locked on Rod — turning it off would only make every fetch fall back to the
// built-in browser path. The server still accepts either value; this gating lives in the
// page. See plan/ai/tools/browser/step-50-fetch-engine-setting-page.md and
// step-51-engine-availability.md.
export function EngineSection({ engine }: { engine: ReturnType<typeof useEngine> }) {
  const { settings, status, error, saving, save, chooseEngine } = engine

  if (!settings) {
    return (
      <section className={cardClassName} style={cardStyle}>
        <h2 className="mb-1 text-base font-semibold text-gray-900">Engine</h2>
        <div className="min-h-[1.2em] text-sm text-red-700">{error || 'Loading…'}</div>
      </section>
    )
  }

  // A backend that predates the setting sends no fetchEngine: that is Rod, the default.
  const useRod = settings.fetchEngine !== 'playwright'
  const playwrightUnavailable = status !== null && !status.playwright.available
  const rodUnavailable = status !== null && !status.rod.available
  // Turning the switch off would select Playwright: only offered when it can run. A stored
  // Playwright choice is always switchable back (switching to Rod must never be blocked).
  const lockedOnRod = useRod && playwrightUnavailable

  return (
    <div className="space-y-5">
      <section className={cardClassName} style={cardStyle}>
        <h2 className="mb-1 text-base font-semibold text-gray-900">Engine</h2>
        <p className="mb-4 text-sm text-gray-500">
          Chooses which engine fetches a page the first time. Rod is the default. Both need Chrome installed, and if the chosen engine
          can&apos;t start, the fetch falls back to the built-in browser path automatically.
        </p>

        <div className="min-h-[1.2em] text-sm text-red-700">{error}</div>

        {!useRod && playwrightUnavailable && (
          <div className="mb-3 rounded-2xl border border-amber-300 bg-amber-50 p-3 text-sm text-amber-900">
            Playwright is selected but it is not available, so every fetch is currently falling back to the built-in browser path.{' '}
            {status.playwright.detail}. Switch back to Rod, or install the Playwright driver.
          </div>
        )}
        {useRod && rodUnavailable && (
          <div className="mb-3 rounded-2xl border border-amber-300 bg-amber-50 p-3 text-sm text-amber-900">
            Rod is selected but it cannot start, so every fetch is currently falling back to the built-in browser path. {status.rod.detail}.
          </div>
        )}

        <div className={`space-y-2 ${lockedOnRod ? 'opacity-90' : ''}`}>
          <Switch
            checked={useRod}
            disabled={saving || lockedOnRod}
            label="Use Rod (recommended)"
            onChange={(on) => {
              chooseEngine(on ? 'rod' : 'playwright')
            }}
          />
          <p className="text-xs text-gray-500">
            On: pages are fetched with Rod — pure Go over the Chrome DevTools Protocol, needs nothing but Chrome. Off: Playwright — it needs
            its own driver (which brings its own Node) installed first; it does not use the Node installed on your computer, and nothing is
            downloaded when it is first used.
          </p>
          {lockedOnRod && (
            <p className="text-xs text-gray-600">
              Playwright can&apos;t be chosen here until its driver is installed: {status.playwright.detail}.
            </p>
          )}
          <p className="text-xs text-gray-500">
            Currently using: <span className="font-medium text-gray-700">{useRod ? 'Rod' : 'Playwright'}</span>. A change applies to the
            next page fetched — no restart needed, and a fetch already running is not interrupted.
          </p>
        </div>
      </section>

      <section className={cardClassName} style={cardStyle}>
        <h2 className="mb-3 text-base font-semibold text-gray-900">When a site asks for a check</h2>
        <div className="space-y-2">
          <Switch
            checked={settings.humanAssist}
            disabled={saving}
            label="Open a browser window for me to clear it (recommended)"
            onChange={(on) => {
              save({ humanAssist: on })
            }}
          />
          <p className="text-xs text-gray-500">
            When an AI agent reaches a site that asks for a check only a person can pass, a Chrome window opens on this computer so you can
            clear it. The agent is told right away that you were asked, so it does not wait. Once you have cleared it, the clearance is kept
            until it expires and the agent&apos;s next call works. The tool never solves the check itself. Off: the agent just gets told
            that the site needs a person.
          </p>
        </div>
      </section>
    </div>
  )
}
