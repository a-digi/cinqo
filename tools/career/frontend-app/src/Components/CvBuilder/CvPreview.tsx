import { useEffect, useRef, useState } from 'react'
import { PDFIcon } from '../../Shared/Icons/icons'
import { previewImages } from './previewImages'
import { previewState } from './cvCardRules'

// The thumbnail of a CV card: the first page of the CV's own PDF, rendered by Cinqo through the
// tool bridge (the tool cannot ship pdf.js) once the card scrolls into view. Until it arrives
// (and whenever it cannot: an older host, a PDF that is gone or unreadable) the template's own
// screenshot is shown, labelled as a sample once the render has given up, and a file glyph when
// the template has none either.
export function CvPreview({
  mediaFileId,
  templateId,
  version,
  title,
}: {
  mediaFileId: string
  templateId: string
  version?: string
  title: string
}) {
  const ref = useRef<HTMLDivElement>(null)
  const [visible, setVisible] = useState(false)
  const [rendered, setRendered] = useState<string | null>(null)
  const [gaveUp, setGaveUp] = useState(false)
  const bridge = window.__cinqoToolBridge
  const canRender = typeof bridge.pdfThumbnail === 'function'

  useEffect(() => {
    const node = ref.current
    if (!node || !canRender) return
    const observer = new IntersectionObserver(
      (entries) => {
        if (entries.some((e) => e.isIntersecting)) {
          setVisible(true)
          observer.disconnect()
        }
      },
      { rootMargin: '200px' },
    )
    observer.observe(node)
    return () => {
      observer.disconnect()
    }
  }, [canRender])

  useEffect(() => {
    if (!visible || !bridge.pdfThumbnail) return
    let cancelled = false
    setGaveUp(false)
    bridge
      .pdfThumbnail(mediaFileId, version)
      .then((url) => {
        if (cancelled) return
        setRendered(url)
        setGaveUp(url === null)
      })
      .catch(() => {
        if (!cancelled) setGaveUp(true)
      })
    return () => {
      cancelled = true
    }
    // bridge is a stable global; the render depends on the file and its version only.
  }, [visible, mediaFileId, version, bridge])

  const templateImage = previewImages[templateId] as string | undefined
  const state = previewState({ rendered, templateImage })

  return (
    <div ref={ref} className="relative h-full w-full bg-gray-100">
      {state === 'pdf' && rendered && <img src={rendered} alt={`Preview of ${title}`} className="h-full w-full object-cover object-top" />}
      {state === 'template' && templateImage && (
        <>
          <img src={templateImage} alt={`${title} (template preview)`} className="h-full w-full object-cover object-top opacity-90" />
          {(gaveUp || !canRender) && (
            <span className="absolute bottom-2 left-2 rounded-full bg-white/90 px-2 py-0.5 text-[10px] font-semibold text-gray-600 shadow-sm">
              Template preview
            </span>
          )}
        </>
      )}
      {state === 'placeholder' && (
        <div className="flex h-full w-full items-center justify-center text-gray-300">
          <PDFIcon className="h-14 w-14" />
        </div>
      )}
    </div>
  )
}
