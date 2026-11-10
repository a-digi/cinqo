import { useCallback, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { ConfirmModal } from '../../Shared/Components/Modal/ConfirmModal'
import { occupiedDialogView, type OccupiedInfo } from './occupiedRules'
import { conversationPath } from './conversationPaths'

// The dialog a start gets when another local model is loaded (plan/ai/conversation/step-241). ask(info) resolves true when the person chose to
// unload the other model and go on, false otherwise. `dialog` must be rendered once in the calling component's JSX.
export function useOccupiedDialog(): { ask: (info: OccupiedInfo) => Promise<boolean>; dialog: React.ReactNode } {
  const [info, setInfo] = useState<OccupiedInfo | null>(null)
  const resolveRef = useRef<(value: boolean) => void>(() => undefined)

  const ask = useCallback((next: OccupiedInfo) => {
    return new Promise<boolean>((resolve) => {
      resolveRef.current = resolve
      setInfo(next)
    })
  }, [])

  const settle = (value: boolean) => {
    resolveRef.current(value)
    setInfo(null)
  }

  const view = info === null ? null : occupiedDialogView(info)
  const dialog = (
    <ConfirmModal
      open={info !== null}
      title={view?.title ?? ''}
      message={view?.message ?? ''}
      details={
        info !== null && info.conversations.length > 0 ? (
          <ul className="space-y-1 text-sm">
            {info.conversations.map((c) => (
              <li key={c.id}>
                <Link
                  to={conversationPath(c.id)}
                  onClick={() => {
                    settle(false)
                  }}
                  className="font-medium text-gray-900 underline"
                >
                  {c.title}
                </Link>
              </li>
            ))}
          </ul>
        ) : undefined
      }
      confirmLabel={view?.unloadLabel ?? undefined}
      cancelLabel={view?.cancelLabel}
      hideConfirm={view !== null && view.unloadLabel === null}
      danger={false}
      onConfirm={() => {
        settle(true)
      }}
      onCancel={() => {
        settle(false)
      }}
    />
  )

  return { ask, dialog }
}
