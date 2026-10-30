// First-page PDF previews, rendered in the browser. pdf.js is loaded the first
// time a PDF tile needs it (never on a page without PDFs). Renders run one at a
// time and each result is cached by file id, so scrolling or re-filtering does
// not fetch or render the same PDF twice. Page 1 only: no scripting, no eval,
// no annotations.
import { mediaDownloadUrl } from '../../../api/media'

const THUMB_WIDTH = 540

type Result = string | null

const cache = new Map<string, Promise<Result>>()
let queue: Promise<unknown> = Promise.resolve()

async function render(id: string): Promise<Result> {
  const pdfjs = await import('pdfjs-dist')
  if (!pdfjs.GlobalWorkerOptions.workerSrc) {
    const worker = await import('pdfjs-dist/build/pdf.worker.min.mjs?url')
    pdfjs.GlobalWorkerOptions.workerSrc = worker.default
  }
  const response = await fetch(mediaDownloadUrl(id), { credentials: 'include' })
  if (!response.ok) return null
  const data = new Uint8Array(await response.arrayBuffer())
  const doc = await pdfjs.getDocument({ data, isEvalSupported: false, enableXfa: false }).promise
  try {
    const page = await doc.getPage(1)
    const base = page.getViewport({ scale: 1 })
    const viewport = page.getViewport({ scale: THUMB_WIDTH / base.width })
    const canvas = document.createElement('canvas')
    canvas.width = Math.floor(viewport.width)
    canvas.height = Math.floor(viewport.height)
    const context = canvas.getContext('2d')
    if (!context) return null
    await page.render({ canvasContext: context, viewport }).promise
    return canvas.toDataURL('image/jpeg', 0.8)
  } finally {
    void doc.destroy()
  }
}

// Resolves to a data URL, or null when the PDF cannot be previewed (corrupt,
// encrypted, unreachable) so the tile keeps its placeholder.
export function pdfThumbnail(id: string): Promise<Result> {
  let result = cache.get(id)
  if (!result) {
    result = new Promise<Result>((resolve) => {
      queue = queue.then(() =>
        render(id).then(resolve, () => {
          resolve(null)
        }),
      )
    })
    cache.set(id, result)
  }
  return result
}

export function forgetPdfThumbnail(id: string): void {
  cache.delete(id)
}
