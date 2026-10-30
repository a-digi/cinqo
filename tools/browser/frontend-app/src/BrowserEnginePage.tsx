import { ChromeSection } from './ChromiumPage'
import { EngineSection } from './EnginePage'
import { HealthStrip } from './HealthStrip'
import { chromeLight, engineLights, headerPill } from './healthRules'
import { PageHeaderCard } from './Shared/PageHeaderCard'
import { ShieldIcon } from './icons'
import { useChromeStatus, useEngine } from './useBrowserStatus'

// "Browser & Engine" — one page for what runs the Browser tool: the Chrome it
// drives (is it installed, and how to install it) and the engine that fetches
// pages with it (Rod or Playwright, availability, and what to do when a site
// asks for a check). It replaces the separate Chromium and Engine pages and is
// the one Browser page that is always reachable: it is where the tool explains
// that nothing else works without Chrome. A health strip above the sections is
// the overview (a light for Chrome and for each engine). While Chrome is missing
// the engine settings are hidden (Rod needs Chrome, and the rest of the tool is
// gated on it too), and they appear as soon as a check finds Chrome. If the
// Chrome status cannot be read the engine section is shown anyway (fail open, as
// the gate does). See plan/ai/tools/browser/step-53-chromium-page-and-gate.md and
// step-50-fetch-engine-setting-page.md.
export function BrowserEnginePage() {
  const chrome = useChromeStatus()
  const engine = useEngine()
  const light = chromeLight(chrome.status, chrome.error, chrome.checking)

  return (
    <div className="browser-v-zoom max-w-5xl p-6 font-sans text-gray-900">
      <PageHeaderCard
        icon={<ShieldIcon />}
        title="Browser &amp; Engine"
        description="The Chrome this tool drives, and the engine that fetches pages with it. Both must be set up on the computer where Cinqo runs."
        pill={headerPill(light)}
      />

      <HealthStrip
        chrome={chrome.status}
        chromeLight={light}
        checking={chrome.checking}
        onCheck={chrome.check}
        engines={engineLights(engine.status, engine.settings)}
        testing={engine.testing}
        testResult={engine.testResult}
        testError={engine.testError}
        onTest={engine.runTest}
      />

      <ChromeSection chrome={chrome} />

      {chrome.status?.installed === false ? (
        <p className="text-sm text-gray-500">The engine settings appear here once Chrome is installed.</p>
      ) : (
        <EngineSection engine={engine} />
      )}
    </div>
  )
}
