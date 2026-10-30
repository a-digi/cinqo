import { useEffect, useState, type ReactNode } from 'react'
import { ShieldIcon } from './icons'
import { ACCENTS, gradient } from './Shared/theme'

// Shown in place of every Browser page except Chromium while no Chrome or
// Chromium is installed (step 53). The menu hides those pages, but every
// route stays registered so a bookmarked or typed URL lands here instead of
// on a page that cannot work. A centred card rather than a page: it says what
// is missing and walks through what to do, using only what Browser & Engine
// already offers (the install steps for this computer, "Check again", Reload).
const BROWSER_ENGINE_PATH = '/tools/browser/browser-engine'

const STEPS = [
  'Open Browser & Engine, which shows how to install Chrome on this computer.',
  'Install Chrome or Chromium on the computer where Cinqo is running.',
  'Press “Check again” there, then reload the page: the rest of the Browser pages appear.',
]

export function ChromeRequired() {
  const rose = ACCENTS[3]
  return (
    <div className="flex justify-center p-6 font-sans text-gray-900">
      <div
        className="mt-6 w-full max-w-xl rounded-3xl border p-7 text-center shadow-sm"
        style={{ borderColor: rose.soft, background: `linear-gradient(160deg, #ffffff 40%, ${rose.soft} 100%)` }}
      >
        <span
          className="mx-auto mb-4 inline-flex h-14 w-14 items-center justify-center rounded-2xl text-white shadow-sm"
          style={{ background: gradient(rose) }}
        >
          <ShieldIcon />
        </span>
        <h1 className="text-xl font-semibold">Chrome is required</h1>
        <p className="mx-auto mt-2 max-w-md text-sm text-gray-600">
          The Browser tool needs Chrome or Chromium installed on the computer where Cinqo is running, and none was found. Until it is
          installed, this page is not available.
        </p>

        <ol className="mx-auto mt-5 max-w-md space-y-2 text-left">
          {STEPS.map((step, i) => (
            <li key={step} className="flex items-start gap-3 rounded-2xl bg-white/80 px-3 py-2 text-sm text-gray-700">
              <span
                aria-hidden="true"
                className="mt-0.5 flex h-5 w-5 shrink-0 items-center justify-center rounded-full text-xs font-bold text-white"
                style={{ background: rose.to }}
              >
                {i + 1}
              </span>
              {step}
            </li>
          ))}
        </ol>

        <a
          href={BROWSER_ENGINE_PATH}
          onClick={(e) => {
            e.preventDefault()
            window.__cinqoToolBridge.navigate(BROWSER_ENGINE_PATH)
          }}
          className="mt-6 inline-block rounded-md bg-gray-800 px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-gray-900"
        >
          Open Browser &amp; Engine
        </a>
      </div>
    </div>
  )
}

// Renders `children` once `chromeReady` resolves true, the notice when it
// resolves false. A component (not a decision made in mount) so unmounting
// before the status arrives cannot leave a stray root behind.
export function ChromeGate({ ready, children }: { ready: Promise<boolean>; children: ReactNode }) {
  const [ok, setOk] = useState<boolean | null>(null)
  useEffect(() => {
    let live = true
    void ready.then((value) => {
      if (live) setOk(value)
    })
    return () => {
      live = false
    }
  }, [ready])
  if (ok === null) return null
  return ok ? <>{children}</> : <ChromeRequired />
}
