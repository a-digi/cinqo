import { useEffect, useRef, useState } from 'react'
import { mediaDownloadUrl, type MediaFile } from '../../../api/media'
import { canPreviewPdf, displayName, mediaKind } from './mediaFilters'
import { pdfThumbnail } from './pdfThumbnail'

function FilePlaceholder({ label }: { label: string }) {
  return (
    <div className="flex h-full w-full flex-col items-center justify-center gap-2 bg-gray-100 text-gray-400">
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.4" className="h-10 w-10">
        <path d="M7 3h7l5 5v13H7z" strokeLinejoin="round" />
        <path d="M14 3v5h5" strokeLinejoin="round" />
      </svg>
      <span className="text-xs font-semibold uppercase tracking-wide">{label}</span>
    </div>
  )
}

// A PDF's first page, rendered once the tile scrolls into view.
function PdfThumbnail({ file }: { file: MediaFile }) {
  const ref = useRef<HTMLDivElement>(null)
  const [src, setSrc] = useState<string | null>(null)
  const [visible, setVisible] = useState(false)
  const previewable = canPreviewPdf(file)

  useEffect(() => {
    const node = ref.current
    if (!node || !previewable) return
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
  }, [previewable])

  useEffect(() => {
    if (!visible) return
    let cancelled = false
    void pdfThumbnail(file.id).then((url) => {
      if (!cancelled) setSrc(url)
    })
    return () => {
      cancelled = true
    }
  }, [visible, file.id])

  return (
    <div ref={ref} className="h-full w-full bg-gray-100">
      {src ? <img src={src} alt={displayName(file)} className="h-full w-full object-cover object-top" /> : <FilePlaceholder label="PDF" />}
    </div>
  )
}

export function MediaThumbnail({ file }: { file: MediaFile }) {
  const kind = mediaKind(file)
  if (kind === 'image') {
    return <img src={mediaDownloadUrl(file.id)} alt={displayName(file)} loading="lazy" className="h-full w-full object-cover" />
  }
  if (kind === 'pdf') return <PdfThumbnail file={file} />
  return <FilePlaceholder label={file.extension.replace(/^\./, '') || 'file'} />
}
