import { useEffect } from 'react'
import { XIcon } from '../Icons/icons'

interface ModalProps {
  open: boolean
  title: string
  onClose: () => void
  children: React.ReactNode
}

// Shared modal — controlled (open/onClose only), no fetch calls, no
// internal form state. Renders nothing while closed rather than
// hiding via CSS, since nothing in this tool needs it to stay mounted
// (no animated-out close, matching the ask: an entrance animation
// only). See plan/ai/tools/career/step-58-portals-page-modal-accordion-redesign.md.
export function Modal({ open, title, onClose, children }: ModalProps) {
  useEffect(() => {
    if (!open) {
      return
    }
    function handleKeyDown(e: KeyboardEvent) {
      if (e.key === 'Escape') {
        onClose()
      }
    }
    window.addEventListener('keydown', handleKeyDown)
    return () => {
      window.removeEventListener('keydown', handleKeyDown)
    }
  }, [open, onClose])

  if (!open) {
    return null
  }

  return (
    // Click-outside-to-dismiss is a mouse-only convenience — Escape
    // (handled above) is the fully equivalent keyboard path, so no
    // onKeyDown is needed here.
    // eslint-disable-next-line jsx-a11y/click-events-have-key-events, jsx-a11y/no-static-element-interactions
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4 [animation:backdrop-in_150ms_ease-out]"
      onClick={onClose}
    >
      {/* Not a real interaction — only stops the backdrop's own onClick
          from treating a click inside the panel as "outside". */}
      {/* eslint-disable-next-line jsx-a11y/click-events-have-key-events, jsx-a11y/no-static-element-interactions */}
      <div
        className="w-full max-w-md rounded-xl border border-gray-200 bg-white shadow-xl [animation:modal-in_180ms_ease-out]"
        onClick={(e) => {
          e.stopPropagation()
        }}
      >
        <div className="flex items-center justify-between border-b border-gray-100 px-5 py-3.5">
          <h2 className="text-sm font-semibold text-gray-900">{title}</h2>
          <button
            type="button"
            onClick={onClose}
            aria-label="Close"
            className="rounded-md p-1 text-gray-400 hover:bg-gray-100 hover:text-gray-700"
          >
            <XIcon />
          </button>
        </div>
        <div className="p-5">{children}</div>
      </div>
    </div>
  )
}
