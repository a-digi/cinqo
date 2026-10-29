import { XIcon } from '../Icons/icons'

interface InfoBoxProps {
  variant: 'success' | 'error' | 'warning'
  // label renders as a small pill above the message — e.g. "Crawl
  // instructions" — entirely caller-supplied text; this component has
  // no opinion on what it says. Omit for a plain, unlabeled box.
  label?: string
  // labelClassName overrides the pill's own background/text color
  // classes (default: tied to variant, same as the message text
  // below it). Set this when the caller has more than one distinct
  // label in play at once (e.g. two different documents that can each
  // independently show their own error) and wants each one's own pill
  // recognizable by color, not just by text — the pill's own identity
  // color, independent of whether this particular instance happens to
  // be an error or a success.
  labelClassName?: string
  message: string
  // Omit to render a non-dismissible box. This component does NOT
  // track "was this dismissed" itself — it always renders when the
  // caller renders it, unconditionally. Deciding whether to render an
  // InfoBox at all for a given message, and remembering that decision
  // across reloads, is entirely the caller's own job. See
  // plan/ai/tools/career/step-70-shared-infobox-component.md.
  onDismiss?: () => void
  // actions (step 90) renders below the message — buttons or links the
  // caller wants offered with it (e.g. "Open the page"). Entirely
  // caller-supplied; this component does not style or wire them.
  actions?: React.ReactNode
  // details (step 90) renders between the message and the actions —
  // secondary text such as the facts behind the message.
  details?: React.ReactNode
}

// Per-variant colors. 'warning' (orange) is for a result that is not an
// error but needs the user's eye — e.g. a crawl that completed and found
// nothing.
const VARIANT_STYLES = {
  success: {
    box: 'border-green-200 bg-green-50',
    label: 'bg-green-100 text-green-800',
    dismiss: 'text-green-400 hover:bg-green-100 hover:text-green-700',
    text: 'text-green-700',
  },
  error: {
    box: 'border-red-200 bg-red-50',
    label: 'bg-red-100 text-red-800',
    dismiss: 'text-red-400 hover:bg-red-100 hover:text-red-700',
    text: 'text-red-700',
  },
  warning: {
    box: 'border-orange-200 bg-orange-50',
    label: 'bg-orange-100 text-orange-800',
    dismiss: 'text-orange-400 hover:bg-orange-100 hover:text-orange-700',
    text: 'text-orange-800',
  },
} as const

// Shared success/error banner — stateless, controlled, no fetch calls,
// same "dumb, reusable" convention Shared/Modal/Modal.tsx already
// established. See plan/ai/tools/career/step-70-shared-infobox-component.md.
export function InfoBox({ variant, label, labelClassName, message, onDismiss, actions, details }: InfoBoxProps) {
  const styles = VARIANT_STYLES[variant]
  return (
    <div className={`rounded-lg border p-2 ${styles.box}`}>
      <div className="flex items-start justify-between gap-2">
        {label ? (
          <span className={`inline-block shrink-0 rounded-full px-2 py-0.5 text-xs font-medium ${labelClassName ?? styles.label}`}>
            {label}
          </span>
        ) : (
          <span />
        )}
        {onDismiss && (
          <button type="button" onClick={onDismiss} aria-label="Dismiss" className={`shrink-0 rounded-md p-0.5 ${styles.dismiss}`}>
            <XIcon />
          </button>
        )}
      </div>
      <p className={`mt-1 text-xs ${styles.text}`}>{message}</p>
      {details && <div className={`mt-1 text-xs opacity-80 ${styles.text}`}>{details}</div>}
      {actions && <div className="mt-2 flex flex-wrap items-center gap-2">{actions}</div>}
    </div>
  )
}
