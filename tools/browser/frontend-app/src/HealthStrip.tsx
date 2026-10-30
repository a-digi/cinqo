import { ChromeIcon, ShieldIcon } from './icons'
import type { ChromeStatus, EngineTestResult } from './api'
import type { EngineLight, Light, Tone } from './healthRules'
import { PAGE_ACCENT } from './Shared/theme'

const DOT: Record<Tone, string> = { ok: 'bg-green-500', bad: 'bg-red-500', unknown: 'bg-gray-300' }
const TEXT: Record<Tone, string> = { ok: 'text-green-700', bad: 'text-red-700', unknown: 'text-gray-500' }

function Tile({ icon, title, light, children }: { icon: React.ReactNode; title: string; light: Light; children: React.ReactNode }) {
  return (
    <div className="min-w-0 rounded-3xl border bg-white p-4 shadow-sm" style={{ borderColor: PAGE_ACCENT.soft }}>
      <div className="mb-3 flex items-center gap-3">
        <span
          className="inline-flex h-10 w-10 shrink-0 items-center justify-center rounded-2xl text-white"
          style={{ background: `linear-gradient(135deg, ${PAGE_ACCENT.from}, ${PAGE_ACCENT.to})` }}
        >
          {icon}
        </span>
        <div className="min-w-0 flex-1">
          <div className="text-sm font-semibold text-gray-900">{title}</div>
          <div className={`flex items-center gap-1.5 text-xs font-medium ${TEXT[light.tone]}`}>
            <span aria-hidden="true" className={`h-2 w-2 rounded-full ${DOT[light.tone]}`} />
            {light.label}
          </div>
        </div>
      </div>
      {children}
    </div>
  )
}

const buttonClass =
  'rounded-full border border-blue-200 bg-white px-3 py-1 text-xs font-medium text-blue-700 hover:bg-blue-50 disabled:cursor-not-allowed disabled:opacity-50'

// The Browser & Engine page's overview: two status tiles side by side above the
// sections: Chrome (a light, its version and where it lives, "Check again") and the
// fetch engine (a light each for Rod and Playwright with the selected one marked, and
// "Test Playwright" with its result).
export function HealthStrip({
  chrome,
  chromeLight,
  checking,
  onCheck,
  engines,
  testing,
  testResult,
  testError,
  onTest,
}: {
  chrome: ChromeStatus | null
  chromeLight: Light
  checking: boolean
  onCheck: () => void
  engines: EngineLight[]
  testing: boolean
  testResult: EngineTestResult | null
  testError: string
  onTest: () => void
}) {
  const engineTone: Tone = engines.some((e) => e.selected && e.tone === 'bad')
    ? 'bad'
    : engines.every((e) => e.tone === 'unknown')
      ? 'unknown'
      : 'ok'
  const selected = engines.find((e) => e.selected)
  const engineLight: Light = {
    tone: engineTone,
    label: engineTone === 'bad' ? `${selected?.name ?? 'Engine'} cannot start` : selected ? `Using ${selected.name}` : 'Unknown',
  }
  return (
    <div className="mb-5 grid gap-4 md:grid-cols-2">
      <Tile icon={<ChromeIcon />} title="Chrome" light={chromeLight}>
        {chrome?.installed && (
          <dl className="space-y-0.5 text-xs text-gray-600">
            {chrome.version && (
              <div className="flex gap-2">
                <dt className="w-14 shrink-0 text-gray-400">Version</dt>
                <dd className="min-w-0 break-words">{chrome.version}</dd>
              </div>
            )}
            <div className="flex gap-2">
              <dt className="w-14 shrink-0 text-gray-400">Location</dt>
              <dd className="min-w-0 break-all font-mono">{chrome.path}</dd>
            </div>
          </dl>
        )}
        {chrome && !chrome.installed && <p className="text-xs text-gray-600">{chrome.detail}</p>}
        <div className="mt-3">
          <button type="button" onClick={onCheck} disabled={checking} className={buttonClass}>
            {checking ? 'Checking…' : 'Check again'}
          </button>
        </div>
      </Tile>

      <Tile icon={<ShieldIcon />} title="Engine" light={engineLight}>
        <ul className="space-y-1.5">
          {engines.map((e) => (
            <li key={e.name} className="flex items-start gap-2 text-xs">
              <span aria-hidden="true" className={`mt-1 h-2 w-2 shrink-0 rounded-full ${DOT[e.tone]}`} />
              <span className="min-w-0">
                <span className="font-medium text-gray-900">{e.name}</span>
                {e.selected && (
                  <span
                    className="ml-1.5 rounded-full px-1.5 py-0.5 text-[10px] font-semibold"
                    style={{ background: PAGE_ACCENT.soft, color: PAGE_ACCENT.text }}
                  >
                    selected
                  </span>
                )}
                <span className={`ml-1.5 ${TEXT[e.tone]}`}>{e.label}</span>
                {e.detail && <span className="block break-words text-gray-500">{e.detail}</span>}
              </span>
            </li>
          ))}
        </ul>
        <div className="mt-3 flex flex-wrap items-center gap-2">
          <button type="button" onClick={onTest} disabled={testing} className={buttonClass}>
            {testing ? 'Testing Playwright…' : 'Test Playwright'}
          </button>
          {testing && <span className="text-xs text-gray-500">Launching Chromium — this can take a few seconds.</span>}
          {testResult && (
            <span role="status" className={`text-xs ${testResult.ok ? 'text-green-700' : 'text-red-700'}`}>
              {testResult.ok ? '✓' : '✗'} {testResult.detail}
              {testResult.ok && ` (${(testResult.durationMs / 1000).toFixed(1)} s)`}
            </span>
          )}
          {testError && <span className="text-xs text-red-700">{testError}</span>}
        </div>
        <p className="mt-2 text-[11px] text-gray-400">
          Availability does not launch a browser, so only the test can tell whether Playwright&apos;s own Chromium is installed.
        </p>
      </Tile>
    </div>
  )
}
