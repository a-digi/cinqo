import { Modal } from '../Modal/Modal'

interface ConfirmationModalProps {
  open: boolean
  title: string
  message: string
  confirmLabel?: string
  cancelLabel?: string
  variant?: 'default' | 'danger'
  onConfirm: () => void
  onCancel: () => void
}

// Shared confirmation dialog — built on top of Shared/Modal/Modal.tsx
// (its own backdrop-click/Escape-to-dismiss already covers "cancel").
// Replaces every window.confirm() call across this frontend: a native
// confirm() blocks synchronously (the caller reads its return value
// immediately), while this is async/callback-based (the caller instead
// tracks "what's pending confirmation" in its own state and acts inside
// onConfirm) — every call site switching to this needs that same
// reshape, not just a drop-in swap. See
// plan/ai/career/shared-components/step-01-confirmation-modal.md.
export function ConfirmationModal({
  open,
  title,
  message,
  confirmLabel = 'Confirm',
  cancelLabel = 'Cancel',
  variant = 'default',
  onConfirm,
  onCancel,
}: ConfirmationModalProps) {
  return (
    <Modal open={open} title={title} onClose={onCancel}>
      <p className="mb-5 text-sm text-gray-600">{message}</p>
      <div className="flex justify-end gap-2">
        <button
          type="button"
          onClick={onCancel}
          className="rounded-md border border-gray-200 px-3.5 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50"
        >
          {cancelLabel}
        </button>
        <button
          type="button"
          onClick={onConfirm}
          className={
            variant === 'danger'
              ? 'rounded-md bg-red-600 px-3.5 py-2 text-sm font-medium text-white hover:bg-red-700'
              : 'rounded-md bg-gray-900 px-3.5 py-2 text-sm font-medium text-white hover:bg-gray-800'
          }
        >
          {confirmLabel}
        </button>
      </div>
    </Modal>
  )
}
