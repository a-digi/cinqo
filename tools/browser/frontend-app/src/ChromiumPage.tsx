import type { useChromeStatus } from './useBrowserStatus'
import type { ChromeStatus } from './api'
import { cardClassName, cardStyle } from './Shared/cardStyle'

// Chrome section of the "Browser & Engine" page (step 53, merged into one page
// with the engine settings) — is a Chrome or Chromium installed on the
// computer where Cinqo (and this tool) runs? Rod and the built-in browser
// path both launch it, so without it the Browser tool cannot fetch pages at
// all. When it is missing this page asks the user to install it and explains
// how for the server's operating system; the tool never installs anything
// itself. The status itself, its light and "Check again" are in the health strip
// above; this card keeps the explanation and the install steps. Once Chrome
// appears after having been missing, the page offers a Reload: a tool's menu is
// registered once when the app loads, so the other Browser pages only show up
// after a reload. Playwright is separate — it launches its own downloaded
// Chromium, not this one. See plan/ai/tools/browser/step-53-chromium-page-and-gate.md.
export function ChromeSection({ chrome }: { chrome: ReturnType<typeof useChromeStatus> }) {
  const { status, error, wasMissing } = chrome
  return (
    <section className={`mb-5 ${cardClassName}`} style={cardStyle}>
      <h2 className="mb-1 text-base font-semibold text-gray-900">Chrome</h2>
      <p className="mb-4 text-sm text-gray-500">
        The Browser tool drives a real Chrome or Chromium to fetch pages. It must be installed on the computer where Cinqo is running.
      </p>

      {error && <div className="mb-3 rounded-2xl border border-red-200 bg-red-50 p-3 text-sm text-red-700">{error}</div>}

      {status?.installed && (
        <p className="text-xs text-gray-500">
          Rod and the built-in browser path both launch this browser. The Playwright engine is separate: it uses its own downloaded
          Chromium.
        </p>
      )}
      {status && !status.installed && <MissingCard status={status} />}
      {!status && !error && <p className="text-sm text-gray-400">Checking…</p>}

      {status?.installed && wasMissing && (
        <div className="mt-4 rounded-2xl border border-green-200 bg-green-50 p-3 text-sm text-green-800">
          Chrome was found. Reload the page to show the rest of the Browser tool&apos;s pages.
          <button
            type="button"
            onClick={() => {
              window.location.reload()
            }}
            className="ml-3 rounded-full bg-green-700 px-3 py-1 text-xs font-medium text-white hover:bg-green-800"
          >
            Reload
          </button>
        </div>
      )}
    </section>
  )
}

function MissingCard({ status }: { status: ChromeStatus }) {
  // The backend says an explicit BROWSER_TOOL_CHROME_PATH override is set but
  // unusable — a different problem from "nothing installed".
  const overrideProblem = status.detail.includes('BROWSER_TOOL_CHROME_PATH')
  return (
    <div className="rounded-2xl border border-red-200 bg-red-50 p-5">
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
