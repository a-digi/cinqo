// Which build of the app a window is running (plan 227, step 2). The app is a single-page app: a window that stays open keeps the JavaScript it was
// loaded with, and a rebuilt app only reaches it after a reload. A build is recognised by the hashed asset files its index.html names
// (Vite puts a content hash into every file name), so two builds differ exactly when their asset names differ; nothing needs to be maintained by hand.

const ASSET = /(?:src|href)\s*=\s*"(\/assets\/[^"]+)"/g

// The asset names of an index.html text, sorted and joined; "" when it names none (a dev server, or a page that is not the app).
export function signatureOfHtml(html: string): string {
  const names: string[] = []
  for (const m of html.matchAll(ASSET)) names.push(m[1])
  return [...new Set(names)].sort().join('\n')
}

// The asset names of the page that is open now, the same way.
export function signatureOfDocument(doc: Document): string {
  const names: string[] = []
  doc.querySelectorAll<HTMLElement>('script[src], link[rel="stylesheet"][href]').forEach((el) => {
    const raw = el.getAttribute('src') ?? el.getAttribute('href') ?? ''
    const path = raw.startsWith('http') ? safePath(raw) : raw
    if (path.startsWith('/assets/')) names.push(path)
  })
  return [...new Set(names)].sort().join('\n')
}

function safePath(url: string): string {
  try {
    return new URL(url).pathname
  } catch {
    return ''
  }
}

// A new build is available when both signatures name assets and they differ. An empty signature on either side means "cannot tell", never "new".
export function isNewBuild(running: string, served: string): boolean {
  return running !== '' && served !== '' && running !== served
}

export const VERSION_CHECK_MS = 60_000
