import { useCallback, useRef, useState } from 'react'
import { ConfirmModal } from './ConfirmModal'

export interface ConfirmOptions {
  title: string
  message: string
  confirmLabel?: string
  cancelLabel?: string
  danger?: boolean
}

// A window.confirm-shaped async replacement — the imperative API every
// existing window.confirm call site adopts with a minimal diff:
// `if (!window.confirm(...))` becomes
// `if (!(await confirm({...})))`, everything else unchanged. `dialog`
// must be rendered once, anywhere in the calling component's own JSX.
// See plan/ai/frontend/frontend/step-07-confirm-modal.md.
export function useConfirm(): {
  confirm: (options: ConfirmOptions) => Promise<boolean>
  dialog: React.ReactNode
} {
  const [options, setOptions] = useState<ConfirmOptions | null>(null)
  const resolveRef = useRef<(value: boolean) => void>(() => {})

  const confirm = useCallback((opts: ConfirmOptions) => {
    return new Promise<boolean>((resolve) => {
      resolveRef.current = resolve
      setOptions(opts)
    })
  }, [])

  const settle = (value: boolean) => {
    resolveRef.current(value)
    setOptions(null)
  }

  const dialog = (
    <ConfirmModal
      open={options !== null}
      title={options?.title ?? ''}
      message={options?.message ?? ''}
      confirmLabel={options?.confirmLabel}
      cancelLabel={options?.cancelLabel}
      danger={options?.danger}
      onConfirm={() => settle(true)}
      onCancel={() => settle(false)}
    />
  )

  return { confirm, dialog }
}
