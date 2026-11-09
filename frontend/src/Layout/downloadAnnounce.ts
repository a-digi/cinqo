// Download feedback for the application window (plan 227, step 3). A Chrome application window has no toolbar, so no download shelf or bubble:
// a correct download would finish with nothing on screen. The shell therefore says it itself, when a download starts. It can only say what is true:
// it started, and that Chrome puts files in the Downloads folder. It cannot know the file is finished (the page is told nothing about that).

export const DOWNLOAD_EVENT = 'cinqo:download-started'

// Something else in the app (a tool, a button that makes a file from data) can announce its own download with
// window.dispatchEvent(new CustomEvent('cinqo:download-started', { detail: { name: 'report.pdf' } })).
export function announceDownload(name: string): void {
  window.dispatchEvent(new CustomEvent(DOWNLOAD_EVENT, { detail: { name } }))
}

const MAX_NAME = 80
const FALLBACK = 'the file'

// A name safe to show as text: one printable line of at most 80 characters, with quotes around it left to the caller.
export function cleanFileName(raw: string): string {
  let out = ''
  for (const ch of raw) {
    const code = ch.codePointAt(0) ?? 0
    if (code < 0x20 || code === 0x7f) continue
    out += ch
  }
  out = out.trim()
  const chars = Array.from(out)
  return chars.length > MAX_NAME ? chars.slice(0, MAX_NAME).join('') : out
}

// The file name a download link will save as: its download attribute, else the last part of its address, else "the file" (an address that ends
// in "download", like Media's, says nothing about the file).
export function downloadNameOf(a: Pick<HTMLAnchorElement, 'getAttribute' | 'href'>): string {
  const attr = cleanFileName(a.getAttribute('download') ?? '')
  if (attr !== '') return attr
  try {
    const last = decodeURIComponent(new URL(a.href).pathname.split('/').filter(Boolean).pop() ?? '')
    const name = cleanFileName(last)
    if (name !== '' && name.toLowerCase() !== 'download') return name
  } catch {
    // an address that cannot be read says nothing
  }
  return FALLBACK
}

export function downloadMessage(name: string): string {
  const n = cleanFileName(name)
  const what = n === '' || n === FALLBACK ? 'Downloading the file.' : `Downloading "${n}".`
  return `${what} Chrome saves it in your Downloads folder.`
}

// The link a click went to, when it is a download link with an address to download from.
export function downloadLinkOf(target: EventTarget | null): HTMLAnchorElement | null {
  if (!(target instanceof Element)) return null
  const a = target.closest('a[download]')
  if (!(a instanceof HTMLAnchorElement)) return null
  const href = a.getAttribute('href') ?? ''
  if (href === '' || href.toLowerCase().startsWith('javascript:')) return null
  return a
}

// The same name twice within this many milliseconds is one download (a double click, or a click that is also announced by the code that made it).
export const DEDUPE_MS = 1500
