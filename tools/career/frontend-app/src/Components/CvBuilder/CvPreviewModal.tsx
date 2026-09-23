import { mediaDownloadUrl } from '../../api'
import { XIcon } from '../../Shared/Icons/icons'

// A full-page PDF preview overlay — no existing full-page modal
// primitive in this bundle (Shared/Modal/Modal.tsx is a small centered
// dialog only), and this frontend is its own separate bundle from the
// core app's own equivalent (Media's step 9 preview modal) — different
// builds entirely, nothing to import across that boundary. Rebuilding
// the same *shape* (full-viewport overlay, PDF in an <iframe>) here is
// what "previewed similar to the PDF preview in the Media" means in
// practice. No image-vs-PDF branching needed — every cv_documents row
// is always a PDF, by construction. See
// plan/ai/career/cv-builder/step-04-frontend-cv-builder.md.
export interface CvPreviewModalProps {
  mediaFileId: string
  title: string
  onClose: () => void
}

export function CvPreviewModal({ mediaFileId, title, onClose }: CvPreviewModalProps) {
  return (
    <div className="fixed inset-0 z-50 flex flex-col bg-white">
      <button
        type="button"
        onClick={onClose}
        aria-label="Close"
        className="absolute right-4 top-4 z-10 rounded-full bg-white p-2 text-gray-500 shadow hover:bg-gray-100 hover:text-gray-700"
      >
        <XIcon />
      </button>
      <iframe title={title} src={mediaDownloadUrl(mediaFileId)} className="h-full w-full" />
    </div>
  )
}
