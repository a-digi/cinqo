import type { useEngine } from './useBrowserStatus'
import { cardClassName, cardStyle } from './Shared/cardStyle'
import { useState } from 'react'
import { CodeIcon, GlobeIcon } from './icons'
import { engineChoice, engineLocked } from './engineRules'
import { Switch } from './Shared/Switch'
import { PAGE_ACCENT } from './Shared/theme'

// Engine section of the "Browser & Engine" page (step 50, availability added in step 51) — two
// checkboxes, Rod (the default) and Playwright, choosing which engine fetches pages. The setting
// holds one engine, so the boxes work as a pair: one is always checked and the way to change it
// is to check the other (unchecking the selected one is refused, with a note). Stored in the same settings object as the Debug toggles and read by the
// backend on every fetch, so the choice applies to the next page fetched, with no
// restart. The whole object is saved on every change (the backend overwrites it on a
// PUT), with the Debug values exactly as they were loaded (see useEngine).
//
// Each engine's availability comes from GET /engine-status and is shown in the health
// strip above. Playwright does NOT use the Node installed in the OS: it needs its own
// driver (which bundles a Node) installed beforehand, so when that is missing its box is
// locked — selecting it would only make every fetch fall back to the
// built-in browser path. The server still accepts either value; this gating lives in the
// page. See plan/ai/tools/browser/step-50-fetch-engine-setting-page.md and
// step-51-engine-availability.md.
export function EngineSection({ engine }: { engine: ReturnType<typeof useEngine> }) {
  const { settings, status, error, saving, save, chooseEngine } = engine
  // Set when the user tried to uncheck the engine that is selected.
  const [needOne, setNeedOne] = useState(false)

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
  const current = useRod ? 'rod' : 'playwright'
  const playwrightAvailable = status === null ? null : status.playwright.available
  // Playwright is only offered when it can run; a stored Playwright choice can always be
  // switched back (choosing Rod must never be blocked).
  const lockedOnRod = engineLocked('playwright', current, playwrightAvailable)

  function onToggle(clicked: 'rod' | 'playwright', checked: boolean) {
    const choice = engineChoice(current, clicked, checked)
    setNeedOne(choice.refused)
    if (!choice.refused && choice.engine !== current) chooseEngine(choice.engine)
  }

  return (
    <div className="browser-stagger space-y-5">
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

        <div className="grid gap-3 md:grid-cols-2" role="group" aria-label="Engine">
          <EngineOption
            name="Rod"
            title="Rod"
            badge="Recommended"
            icon={<CodeIcon className="h-5 w-5" />}
            checked={useRod}
            disabled={saving}
            onChange={(checked) => {
              onToggle('rod', checked)
            }}
          >
            Pure Go over the Chrome DevTools Protocol. Needs nothing but Chrome.
          </EngineOption>
          <EngineOption
            name="Playwright"
            title="Playwright"
            badge={lockedOnRod ? 'Driver missing' : undefined}
            icon={<GlobeIcon className="h-5 w-5" />}
            checked={!useRod}
            disabled={saving || lockedOnRod}
            onChange={(checked) => {
              onToggle('playwright', checked)
            }}
          >
            Needs its own driver (which brings its own Node), not the Node on your computer. Nothing is downloaded when it is first used.
          </EngineOption>
        </div>

        {needOne && (
          <p role="status" className="mt-3 text-xs font-medium text-amber-800">
            One engine has to be selected. Check the other one to switch.
          </p>
        )}
        {lockedOnRod && (
          <p className="mt-3 text-xs text-gray-600">
            Playwright can&apos;t be chosen here until its driver is installed: {status?.playwright.detail}.
          </p>
        )}
        <p className="mt-3 text-xs text-gray-500">
          Currently using: <span className="font-medium text-gray-700">{useRod ? 'Rod' : 'Playwright'}</span>. A change applies to the next
          page fetched — no restart needed, and a fetch already running is not interrupted.
        </p>
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

// One engine's checkbox tile: half of the row, with an icon, the name, an optional badge and a short
// description, and a large checkbox at the top right. Blue while it is the selected engine.
function EngineOption({
  name,
  title,
  badge,
  icon,
  checked,
  disabled,
  onChange,
  children,
}: {
  name: string
  title: string
  badge?: string
  icon: React.ReactNode
  checked: boolean
  disabled: boolean
  onChange: (checked: boolean) => void
  children: React.ReactNode
}) {
  return (
    // aria-label given explicitly — the visible title is nested two <span> levels deep, past what
    // jsx-a11y's own accessible-text detection reliably walks.
    <label
      aria-label={title}
      className={`flex h-full cursor-pointer flex-col gap-3 rounded-2xl border-2 p-4 transition-colors ${
        disabled && !checked ? 'cursor-not-allowed opacity-60' : 'hover:border-blue-300'
      }`}
      style={checked ? { borderColor: PAGE_ACCENT.to, background: PAGE_ACCENT.soft } : { borderColor: '#e5e7eb', background: '#fff' }}
    >
      <span className="flex items-start gap-3">
        <span
          className="inline-flex h-10 w-10 shrink-0 items-center justify-center rounded-xl text-white"
          style={checked ? { background: `linear-gradient(135deg, ${PAGE_ACCENT.from}, ${PAGE_ACCENT.to})` } : { background: '#cbd5e1' }}
        >
          {icon}
        </span>
        <span className="min-w-0 flex-1">
          <span className="block text-base font-semibold text-gray-900">{title}</span>
          {badge && (
            <span
              className="mt-0.5 inline-block rounded-full px-2 py-0.5 text-[11px] font-semibold"
              style={{ background: checked ? '#fff' : PAGE_ACCENT.soft, color: PAGE_ACCENT.text }}
            >
              {badge}
            </span>
          )}
        </span>
        <input
          type="checkbox"
          name={name}
          checked={checked}
          disabled={disabled}
          onChange={(e) => {
            onChange(e.target.checked)
          }}
          className="mt-0.5 h-5 w-5 shrink-0 rounded border-gray-300 accent-blue-600"
        />
      </span>
      <span className="block text-sm text-gray-600">{children}</span>
      <span className="mt-auto block text-xs font-semibold" style={{ color: checked ? PAGE_ACCENT.text : '#94a3b8' }}>
        {checked ? '✓ Selected' : 'Not selected'}
      </span>
    </label>
  )
}
