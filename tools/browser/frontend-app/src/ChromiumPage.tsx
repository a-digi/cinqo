import { useCallback, useEffect, useState } from 'react'
import { fetchChromeStatus, type ChromeStatus } from './api'

// Chrome section of the "Browser & Engine" page (step 53, merged into one page
// with the engine settings) — is a Chrome or Chromium installed on the
// computer where Cinqo (and this tool) runs? Rod and the built-in browser
// path both launch it, so without it the Browser tool cannot fetch pages at
// all. When it is missing this page asks the user to install it and explains
// how for the server's operating system; the tool never installs anything
// itself. "Check again" re-reads the status without restarting anything.
// Once Chrome appears after having been missing, the page offers a Reload:
// a tool's menu is registered once when the app loads, so the other Browser
// pages only show up after a reload. Playwright is separate — it launches its
// own downloaded Chromium, not this one. See
// plan/ai/tools/browser/step-53-chromium-page-and-gate.md.
// onStatus reports whether Chrome is installed (true/false), or null when the
// status could not be read, so the page around this can decide what else to show.
export function ChromeSection({ onStatus }: { onStatus?: (installed: boolean | null) => void }) {
  const [status, setStatus] = useState<ChromeStatus | null>(null)
  const [error, setError] = useState('')
  const [checking, setChecking] = useState(true)
  // True once this page has seen Chrome missing — so that finding it later
  // (after the user installed it) can offer the Reload.
  const [wasMissing, setWasMissing] = useState(false)

  // Stable as long as onStatus is (a state setter is), so the effect below runs
  // once on mount and Check again re-runs the same function.
  const check = useCallback(() => {
    setChecking(true)
    setError('')
    fetchChromeStatus()
      .then((next) => {
        setStatus(next)
        if (!next.installed) setWasMissing(true)
        onStatus?.(next.installed)
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
        onStatus?.(null)
      })
      .finally(() => {
        setChecking(false)
      })
  }, [onStatus])

  useEffect(() => {
    check()
  }, [check])

  return (
    <section className="mb-10">
      <h2 className="mb-1.5 text-lg font-semibold">Chrome</h2>
      <p className="mb-5 text-sm text-gray-500">
        The Browser tool drives a real Chrome or Chromium to fetch pages. It must be installed on the computer where Cinqo is running.
      </p>

      {error && <div className="mb-3 rounded-md border border-red-200 bg-red-50 p-3 text-sm text-red-700">{error}</div>}

      {status?.installed && <InstalledCard status={status} />}
      {status && !status.installed && <MissingCard status={status} />}
      {!status && !error && <p className="text-sm text-gray-400">Checking…</p>}

      {status?.installed && wasMissing && (
        <div className="mt-4 rounded-md border border-green-200 bg-green-50 p-3 text-sm text-green-800">
          Chrome was found. Reload the page to show the rest of the Browser tool&apos;s pages.
          <button
            type="button"
            onClick={() => {
              window.location.reload()
            }}
            className="ml-3 rounded-md bg-green-700 px-3 py-1 text-xs font-medium text-white hover:bg-green-800"
          >
            Reload
          </button>
        </div>
      )}

      <div className="mt-4">
        <button
          type="button"
          onClick={check}
          disabled={checking}
          className="rounded-md border border-gray-300 bg-white px-3 py-1.5 text-sm font-medium text-gray-800 hover:bg-gray-50 disabled:cursor-not-allowed disabled:opacity-50"
        >
          {checking ? 'Checking…' : 'Check again'}
        </button>
      </div>
    </section>
  )
}

function InstalledCard({ status }: { status: ChromeStatus }) {
  return (
    <div className="rounded-md border border-green-200 bg-green-50 p-5 shadow-sm">
      <p className="mb-2 flex items-center gap-2 text-sm font-medium text-green-800">
        <span className="rounded-full bg-green-100 px-2 py-0.5 text-xs">Installed</span>
        Chrome or Chromium is installed.
      </p>
      <dl className="space-y-1 text-xs text-gray-700">
        {status.version && (
          <div className="flex gap-2">
            <dt className="w-16 shrink-0 text-gray-500">Version</dt>
            <dd>{status.version}</dd>
          </div>
        )}
        <div className="flex gap-2">
          <dt className="w-16 shrink-0 text-gray-500">Location</dt>
          <dd className="break-all font-mono">{status.path}</dd>
        </div>
      </dl>
      <p className="mt-3 text-xs text-gray-500">
        Rod and the built-in browser path both launch this browser. The Playwright engine is separate: it uses its own downloaded Chromium.
      </p>
    </div>
  )
}

function MissingCard({ status }: { status: ChromeStatus }) {
  // The backend says an explicit BROWSER_TOOL_CHROME_PATH override is set but
  // unusable — a different problem from "nothing installed".
  const overrideProblem = status.detail.includes('BROWSER_TOOL_CHROME_PATH')
  return (
    <div className="rounded-md border border-red-200 bg-red-50 p-5 shadow-sm">
      <p className="mb-2 flex items-center gap-2 text-sm font-medium text-red-800">
        <span className="rounded-full bg-red-100 px-2 py-0.5 text-xs">Not installed</span>
        {overrideProblem ? 'The configured Chrome cannot be used.' : 'Chrome or Chromium is not installed.'}
      </p>
      <p className="mb-3 text-sm text-red-800">
        {status.detail}. Without it the Browser tool cannot fetch any page, so its other pages are hidden until Chrome is installed.
      </p>
      {overrideProblem ? (
        <p className="text-xs text-gray-700">
          The environment variable <span className="font-mono">BROWSER_TOOL_CHROME_PATH</span> points at a file that does not exist or is
          not a program. Fix the path, or remove the variable to let the tool find Chrome by itself, then restart Cinqo and check again.
        </p>
      ) : (
        <>
          <p className="mb-2 text-sm font-medium text-gray-800">Please install Chrome on the computer where Cinqo is running:</p>
          <InstallInstructions os={status.os} />
        </>
      )}
    </div>
  )
}

function Command({ children }: { children: string }) {
  return <code className="block overflow-x-auto rounded bg-gray-900 px-3 py-1.5 font-mono text-xs text-gray-100">{children}</code>
}

function ChromeLink() {
  return (
    <a
      href="https://www.google.com/chrome/"
      target="_blank"
      rel="noopener noreferrer"
      className="font-medium text-blue-700 underline hover:text-blue-900"
    >
      google.com/chrome
    </a>
  )
}

// Install instructions for the SERVER's operating system (status.os, a Go
// GOOS value). Static text: nothing is downloaded or run on the user's behalf.
function InstallInstructions({ os }: { os: string }) {
  if (os === 'darwin') {
    return (
      <div className="space-y-2 text-xs text-gray-700">
        <p>
          Download and install it from <ChromeLink />, or with Homebrew:
        </p>
        <Command>brew install --cask google-chrome</Command>
      </div>
    )
  }
  if (os === 'windows') {
    return (
      <div className="space-y-2 text-xs text-gray-700">
        <p>
          Download and install it from <ChromeLink />, or with winget:
        </p>
        <Command>winget install Google.Chrome</Command>
      </div>
    )
  }
  if (os === 'linux') {
    return (
      <div className="space-y-2 text-xs text-gray-700">
        <p>
          Install Google Chrome from <ChromeLink /> (a .deb or .rpm package), or Chromium from your distribution:
        </p>
        <p>Debian or Ubuntu:</p>
        <Command>sudo apt install chromium</Command>
        <p>Fedora:</p>
        <Command>sudo dnf install chromium</Command>
      </div>
    )
  }
  return (
    <p className="text-xs text-gray-700">
      Install Google Chrome from <ChromeLink /> or your system&apos;s Chromium package.
    </p>
  )
}
