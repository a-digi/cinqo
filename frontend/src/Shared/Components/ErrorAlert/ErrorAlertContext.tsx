import { createContext, useCallback, useContext, useState, type ReactNode } from 'react'

export interface ErrorAlertItem {
  id: string
  message: string
}

export interface ErrorAlertContextProps {
  errors: ErrorAlertItem[]
  // Show a persistent error alert. Unlike the SnackBar/Toast it never
  // auto-dismisses — it stays until the user closes it (or clearErrors runs).
  showError: (message: string) => void
  dismissError: (id: string) => void
  // Remove any alert whose text matches — lets a caller that surfaces a known
  // message (e.g. a form validation error) retract it once resolved, without
  // tracking the generated id. Idempotent.
  dismissMessage: (message: string) => void
  clearErrors: () => void
}

const ErrorAlertContext = createContext<ErrorAlertContextProps | null>(null)

// Monotonic id source — avoids Date.now()/Math.random() and keeps keys stable.
let counter = 0

// Holds the app-wide stack of persistent error alerts. Rendered by the Layout
// (via an <ErrorAlert/> presentational component, added in frontend/step-04
// once Layout.tsx exists) directly below the top bar. Sibling to
// SnackBarProvider: the SnackBar is for transient feedback, this is for
// errors that must stay put until acknowledged.
export function ErrorAlertProvider({ children }: { children: ReactNode }) {
  const [errors, setErrors] = useState<ErrorAlertItem[]>([])

  const showError = useCallback((message: string) => {
    const text = message.trim()
    if (!text) return
    setErrors((prev) => {
      // Don't stack an identical message that is already visible.
      if (prev.some((e) => e.message === text)) return prev
      counter += 1
      return [...prev, { id: `err-${counter}`, message: text }]
    })
  }, [])

  const dismissError = useCallback((id: string) => {
    setErrors((prev) => prev.filter((e) => e.id !== id))
  }, [])

  const dismissMessage = useCallback((message: string) => {
    const text = message.trim()
    setErrors((prev) => prev.filter((e) => e.message !== text))
  }, [])

  const clearErrors = useCallback(() => setErrors([]), [])

  return (
    <ErrorAlertContext.Provider value={{ errors, showError, dismissError, dismissMessage, clearErrors }}>
      {children}
    </ErrorAlertContext.Provider>
  )
}

export function useErrorAlert(): ErrorAlertContextProps {
  const ctx = useContext(ErrorAlertContext)
  if (!ctx) {
    throw new Error('useErrorAlert must be used within an ErrorAlertProvider')
  }
  return ctx
}
