import { XIcon } from '../Icons/icons'

interface InfoBoxProps {
  variant: 'success' | 'error'
  // label renders as a small pill before the message — e.g. "Crawl
  // instructions" — entirely caller-supplied text; this component has
  // no opinion on what it says. Omit for a plain, unlabeled box.
  label?: string
  message: string
  // Omit to render a non-dismissible box. This component does NOT
  // track "was this dismissed" itself — it always renders when the
  // caller renders it, unconditionally. Deciding whether to render an
  // InfoBox at all for a given message, and remembering that decision
  // across reloads, is entirely the caller's own job. See
  // plan/ai/tools/career/step-70-shared-infobox-component.md.
  onDismiss?: () => void
}

// Shared success/error banner — stateless, controlled, no fetch calls,
// same "dumb, reusable" convention Shared/Modal/Modal.tsx already
// established. See plan/ai/tools/career/step-70-shared-infobox-component.md.
export function InfoBox({ variant, label, message, onDismiss }: InfoBoxProps) {
  const isError = variant === 'error'
  return (
    <div
      className={`flex flex-wrap items-start gap-2 rounded-lg border p-2 ${
        isError ? 'border-red-200 bg-red-50' : 'border-green-200 bg-green-50'
      }`}
    >
      {label && (
        <span
          className={`inline-block shrink-0 rounded-full px-2 py-0.5 text-xs font-medium ${
            isError ? 'bg-red-100 text-red-800' : 'bg-green-100 text-green-800'
          }`}
        >
          {label}
        </span>
      )}
      <p className={`flex-1 text-xs ${isError ? 'text-red-700' : 'text-green-700'}`}>{message}</p>
      {onDismiss && (
        <button
          type="button"
          onClick={onDismiss}
          aria-label="Dismiss"
          className={`ml-auto shrink-0 rounded-md p-0.5 ${
            isError ? 'text-red-400 hover:bg-red-100 hover:text-red-700' : 'text-green-400 hover:bg-green-100 hover:text-green-700'
          }`}
        >
          <XIcon />
        </button>
      )}
    </div>
  )
}
