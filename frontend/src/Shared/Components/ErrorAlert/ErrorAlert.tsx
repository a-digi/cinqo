import type { ErrorAlertItem } from './ErrorAlertContext'

export interface ErrorAlertProps {
  errors: ErrorAlertItem[]
  onDismiss: (id: string) => void
}

// A stack of persistent, closable error alerts — the error counterpart to the
// SnackBar/Toast, but it never auto-dismisses. Pure presentational: state comes
// in via props, closing goes out via onDismiss; no API calls. Renders nothing
// when there are no errors. Rendered by Layout directly below the Navbar.
export function ErrorAlert({ errors, onDismiss }: ErrorAlertProps) {
  if (errors.length === 0) return null

  return (
    <div className="shrink-0 space-y-2 border-b border-red-200 bg-red-50 px-6 py-3">
      {errors.map((e) => (
        <div key={e.id} role="alert" className="flex items-start gap-3 text-sm text-red-800">
          <svg className="mt-0.5 h-5 w-5 shrink-0 text-red-600" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.8}>
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              d="M12 9v3.75m0 3.75h.008M10.29 3.86 1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0Z"
            />
          </svg>
          <span className="flex-1 pt-0.5">{e.message}</span>
          <button
            type="button"
            onClick={() => {
              onDismiss(e.id)
            }}
            aria-label="Close"
            className="shrink-0 rounded p-0.5 text-red-500 transition-colors hover:bg-red-100 hover:text-red-700"
          >
            <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M6 18 18 6M6 6l12 12" />
            </svg>
          </button>
        </div>
      ))}
    </div>
  )
}
