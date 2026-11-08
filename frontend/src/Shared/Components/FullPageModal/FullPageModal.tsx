import { XIcon } from '../IconButton/icons'

// A plain, full-viewport overlay — distinct in shape from
// Modal/ConfirmModal.tsx's own small centered dialog. Purely
// presentational (no API calls, no context reads beyond none): props
// in, callback out. First used by Media's own preview+metadata modal
// (plan/ai/media/step-09-title-metadata-and-preview.md), but generic
// enough for any future feature needing a full-page overlay.
export interface FullPageModalProps {
  onClose: () => void
  children: React.ReactNode
}

export function FullPageModal({ onClose, children }: FullPageModalProps) {
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
      {children}
    </div>
  )
}
