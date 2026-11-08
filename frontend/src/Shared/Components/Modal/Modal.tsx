import { useEffect, useRef } from 'react'

// A plain content dialog (plan 217): a title, anything as content, one Close button. Escape and a click on the backdrop close it, Close is focused
// when it opens. Hand-rolled against Tailwind utility classes like ConfirmModal, which stays the one for questions that need an answer.
export interface ModalProps {
  open: boolean
  title: string
  children: React.ReactNode
  onClose: () => void
  closeLabel?: string
}

export function Modal({ open, title, children, onClose, closeLabel = 'Close' }: ModalProps) {
  const closeRef = useRef<HTMLButtonElement>(null)

  useEffect(() => {
    if (!open) return
    closeRef.current?.focus()
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
    }
    window.addEventListener('keydown', onKey)
    return () => {
      window.removeEventListener('keydown', onKey)
    }
  }, [open, onClose])

  if (!open) return null

  return (
    // Click-outside-to-dismiss is a mouse-only convenience; Escape (above) is the keyboard path.
    // eslint-disable-next-line jsx-a11y/click-events-have-key-events, jsx-a11y/no-static-element-interactions
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4" onClick={onClose}>
      {/* Only stops a click inside the dialog from counting as "outside". */}
      {/* eslint-disable-next-line jsx-a11y/click-events-have-key-events, jsx-a11y/no-noninteractive-element-interactions */}
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby="content-modal-title"
        className="flex max-h-[85vh] w-full max-w-xl flex-col rounded-2xl bg-white shadow-xl"
        onClick={(e) => {
          e.stopPropagation()
        }}
      >
        <h2 id="content-modal-title" className="border-b border-gray-100 px-5 py-4 text-base font-semibold text-gray-900">
          {title}
        </h2>
        <div className="min-h-0 flex-1 overflow-y-auto px-5 py-4">{children}</div>
        <div className="flex justify-end border-t border-gray-100 px-5 py-3">
          <button
            ref={closeRef}
            type="button"
            onClick={onClose}
            className="rounded-md border border-gray-300 px-3 py-1.5 text-sm text-gray-700 hover:bg-gray-50"
          >
            {closeLabel}
          </button>
        </div>
      </div>
    </div>
  )
}
