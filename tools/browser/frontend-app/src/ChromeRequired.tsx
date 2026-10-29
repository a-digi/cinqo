import { useEffect, useState, type ReactNode } from 'react'

// Shown in place of every Browser page except Chromium while no Chrome or
// Chromium is installed (step 53). The menu hides those pages, but every
// route stays registered so a bookmarked or typed URL lands here instead of
// on a page that cannot work.
export function ChromeRequired() {
  return (
    <div className="max-w-2xl p-6 font-sans text-gray-900">
      <h1 className="mb-1.5 text-xl font-semibold">Chrome is required</h1>
      <div className="rounded-md border border-red-200 bg-red-50 p-5 text-sm text-red-800 shadow-sm">
        <p className="mb-3">
          The Browser tool needs Chrome or Chromium installed on the computer where Cinqo is running, and none was found. Until it is
          installed, this page is not available.
        </p>
        <a href="/tools/browser/chromium" className="font-medium text-blue-700 underline hover:text-blue-900">
          Open the Chromium page
        </a>
        <span className="text-gray-700"> to see how to install it.</span>
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
