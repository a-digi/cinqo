import { useState } from 'react'
import { ChromeSection } from './ChromiumPage'
import { EngineSection } from './EnginePage'

// "Browser & Engine" — one page for what runs the Browser tool: the Chrome it
// drives (is it installed, and how to install it) and the engine that fetches
// pages with it (Rod or Playwright, availability, and what to do when a site
// asks for a check). It replaces the separate Chromium and Engine pages and is
// the one Browser page that is always reachable: it is where the tool explains
// that nothing else works without Chrome. While Chrome is missing the engine
// settings are hidden (Rod needs Chrome, and the rest of the tool is gated on
// it too), and they appear as soon as a check finds Chrome. If the Chrome
// status cannot be read the engine section is shown anyway (fail open, as the
// gate does). See plan/ai/tools/browser/step-53-chromium-page-and-gate.md and
// step-50-fetch-engine-setting-page.md.
export function BrowserEnginePage() {
  const [chromeInstalled, setChromeInstalled] = useState<boolean | null>(null)

  return (
    <div className="max-w-2xl p-6 font-sans text-gray-900">
      <h1 className="mb-1.5 text-xl font-semibold">Browser &amp; Engine</h1>
      <p className="mb-6 text-sm text-gray-500">
        The Chrome this tool drives, and the engine that fetches pages with it. Both must be set up on the computer where Cinqo runs.
      </p>

      <ChromeSection onStatus={setChromeInstalled} />

      {chromeInstalled === false ? (
        <p className="text-sm text-gray-500">The engine settings appear here once Chrome is installed.</p>
      ) : (
        <EngineSection />
      )}
    </div>
  )
}
