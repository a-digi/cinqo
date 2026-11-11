import { useEffect, useRef } from 'react'
import { createPortal } from 'react-dom'
import { XIcon } from '../Icons/icons'

interface ModalProps {
  open: boolean
  title: string
  onClose: () => void
  children: React.ReactNode
  // 'wide' is for a long form: a wider panel whose body scrolls inside the window. The default
  // is unchanged. 'full' (plan/ai/tools/career/step-245-instruction-modals-full-page.md) is the whole browser window on a white
  // page: it is rendered outside the item that opened it (a portal into document.body), has a header with Close and a body that
  // scrolls on its own, stops the page behind from scrolling and hands the focus back when it closes.
  size?: 'default' | 'wide' | 'full'
  // Full size only (plan 246): a line under the title (what is being edited) and a bar fixed at the bottom (the actions), outside the scrolling body.
  subtitle?: React.ReactNode
  footer?: React.ReactNode
}

// Shared modal — controlled (open/onClose only), no fetch calls, no
// internal form state. Renders nothing while closed rather than
// hiding via CSS, since nothing in this tool needs it to stay mounted
// (no animated-out close, matching the ask: an entrance animation
// only). See plan/ai/tools/career/step-58-portals-page-modal-accordion-redesign.md.
export function Modal({ open, title, onClose, children, size = 'default', subtitle, footer }: ModalProps) {
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

  const isFull = open && size === 'full'
  const pageRef = useRef<HTMLDivElement>(null)

  // The page behind a full-page modal does not scroll while it is open; what was set before is put back on every way out (close, unmount).
  useEffect(() => {
    if (!isFull) return
    const before = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    return () => {
      document.body.style.overflow = before
    }
  }, [isFull])

  // The focus goes into a full-page modal when it opens and back to what had it (the button that opened it) when it closes.
  useEffect(() => {
    if (!isFull) return
    const opener = document.activeElement instanceof HTMLElement ? document.activeElement : null
    pageRef.current?.focus()
    return () => {
      opener?.focus()
    }
  }, [isFull])

  if (!open) {
    return null
  }

  if (size === 'full') {
    return createPortal(
      <div
        ref={pageRef}
        role="dialog"
        aria-modal="true"
        aria-label={title}
        tabIndex={-1}
        className="fixed inset-0 z-[60] flex flex-col bg-white font-sans text-gray-900 outline-none"
      >
        <div className="flex shrink-0 items-center justify-between gap-4 border-b border-gray-200 px-6 py-3.5">
          <div className="min-w-0">
            <h2 className="text-base font-semibold text-gray-900">{title}</h2>
            {subtitle !== undefined && subtitle !== null && (
              <div className="mt-1 flex flex-wrap items-center gap-2 text-xs text-gray-500">{subtitle}</div>
            )}
          </div>
          <button
            type="button"
            onClick={onClose}
            aria-label="Close"
            className="rounded-lg p-2 text-gray-400 hover:bg-gray-100 hover:text-gray-700"
          >
            {/* twice the size of the other dialogs' close button (plan 246 follow-up): the window is the whole screen, the way out is easy to hit */}
            <XIcon className="h-7 w-7" />
          </button>
        </div>
        <div className="min-h-0 flex-1 overflow-y-auto px-6 py-5">{children}</div>
        {footer !== undefined && footer !== null && <div className="shrink-0 border-t border-gray-200 bg-white px-6 py-3">{footer}</div>}
      </div>,
      document.body,
    )
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
        className={`flex w-full flex-col rounded-xl border border-gray-200 bg-white shadow-xl [animation:modal-in_180ms_ease-out] ${
          size === 'wide' ? 'max-h-[90vh] max-w-3xl' : 'max-w-md'
        }`}
        onClick={(e) => {
          e.stopPropagation()
        }}
      >
        <div className="flex shrink-0 items-center justify-between border-b border-gray-100 px-5 py-3.5">
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
        <div className={`p-5 ${size === 'wide' ? 'overflow-y-auto' : ''}`}>{children}</div>
      </div>
    </div>
  )
}
