import { createContext, useCallback, useContext, useRef, useState, type ReactNode } from 'react'

export type SnackBarVariant = 'info' | 'danger' | 'success' | 'error'
export type SnackBarPosition = 'top-right' | 'bottom-right'

const DEFAULT_DURATION_MS = 4000
const DEFAULT_POSITION: SnackBarPosition = 'top-right'

interface SnackBarMessage {
  id: string
  text: string
  variant: SnackBarVariant
  position: SnackBarPosition
}

export interface SnackBarContextProps {
  infoMessage: (message: string, duration?: number, position?: SnackBarPosition) => void
  dangerMessage: (message: string, duration?: number, position?: SnackBarPosition) => void
  successMessage: (message: string, duration?: number, position?: SnackBarPosition) => void
  errorMessage: (message: string, duration?: number, position?: SnackBarPosition) => void
  removeMessage: (id: string) => void
}

const SnackBarContext = createContext<SnackBarContextProps | null>(null)

// Styling here is a minimal, easily-revisable placeholder, not a final
// design-system decision.
const VARIANT_CLASSES: Record<SnackBarVariant, string> = {
  info: 'bg-slate-800 text-white',
  danger: 'bg-red-700 text-white',
  error: 'bg-red-700 text-white',
  success: 'bg-green-700 text-white',
}

const POSITION_CLASSES: Record<SnackBarPosition, string> = {
  'top-right': 'top-4 right-4 items-end',
  'bottom-right': 'bottom-4 right-4 items-end',
}

export function SnackBarProvider({ children }: { children: ReactNode }) {
  const [messages, setMessages] = useState<SnackBarMessage[]>([])
  const timers = useRef<Map<string, ReturnType<typeof setTimeout>>>(new Map())

  const removeMessage = useCallback((id: string) => {
    setMessages((prev) => prev.filter((m) => m.id !== id))
    const timer = timers.current.get(id)
    if (timer) {
      clearTimeout(timer)
      timers.current.delete(id)
    }
  }, [])

  const pushMessage = useCallback(
    (text: string, variant: SnackBarVariant, duration = DEFAULT_DURATION_MS, position = DEFAULT_POSITION) => {
      const id = `${Date.now()}-${Math.random().toString(36).slice(2)}`
      setMessages((prev) => [...prev, { id, text, variant, position }])
      const timer = setTimeout(() => {
        removeMessage(id)
      }, duration)
      timers.current.set(id, timer)
    },
    [removeMessage],
  )

  const value: SnackBarContextProps = {
    infoMessage: (message, duration, position) => {
      pushMessage(message, 'info', duration, position)
    },
    dangerMessage: (message, duration, position) => {
      pushMessage(message, 'danger', duration, position)
    },
    successMessage: (message, duration, position) => {
      pushMessage(message, 'success', duration, position)
    },
    errorMessage: (message, duration, position) => {
      pushMessage(message, 'error', duration, position)
    },
    removeMessage,
  }

  const messagesByPosition = (position: SnackBarPosition) => messages.filter((m) => m.position === position)

  return (
    <SnackBarContext.Provider value={value}>
      {children}
      {(['top-right', 'bottom-right'] as SnackBarPosition[]).map((position) => (
        <div key={position} className={`pointer-events-none fixed z-50 flex flex-col gap-2 ${POSITION_CLASSES[position]}`}>
          {messagesByPosition(position).map((m) => (
            <div
              key={m.id}
              role="status"
              className={`pointer-events-auto rounded-md px-4 py-2 text-sm shadow-lg ${VARIANT_CLASSES[m.variant]}`}
            >
              {m.text}
            </div>
          ))}
        </div>
      ))}
    </SnackBarContext.Provider>
  )
}

// Context + hook co-located deliberately (same convention as every other
// *Context.tsx in this codebase) — costs this file Fast Refresh for the
// hook specifically, never a runtime issue.
// eslint-disable-next-line react-refresh/only-export-components
export function useSnackBar(): SnackBarContextProps {
  const ctx = useContext(SnackBarContext)
  if (!ctx) {
    throw new Error('useSnackBar must be used within a SnackBarProvider')
  }
  return ctx
}
