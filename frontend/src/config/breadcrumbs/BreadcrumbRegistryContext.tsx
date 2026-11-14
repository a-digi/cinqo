import { createContext, useCallback, useContext, useMemo, useRef, useState, type ReactNode } from 'react'
import { addRegistration, type BreadcrumbRegistration } from './breadcrumbRules'
import { SYSTEM_BREADCRUMBS } from './systemBreadcrumbs'

export interface BreadcrumbRegistryValue {
  // Every trail registered so far: the system's first, then the tools' in the order they registered.
  registrations: readonly BreadcrumbRegistration[]
  // Adds one (a tool's, through the bridge). A bad or repeated path is refused with a console error and changes nothing.
  register: (registration: BreadcrumbRegistration) => void
}

const BreadcrumbRegistryContext = createContext<BreadcrumbRegistryValue>({ registrations: [], register: () => undefined })

// Context + hook co-located like every other *Context.tsx here (costs Fast Refresh for the hook only).
// eslint-disable-next-line react-refresh/only-export-components
export function useBreadcrumbRegistry(): BreadcrumbRegistryValue {
  return useContext(BreadcrumbRegistryContext)
}

// Holds the registered trails. It wraps ToolRegistryProvider, which hands a tool's registerBreadcrumbs calls to `register`.
// `system` is a prop only so a test can pass its own; the app uses the shell's list.
export function BreadcrumbRegistryProvider({
  children,
  system = SYSTEM_BREADCRUMBS,
}: {
  children: ReactNode
  system?: readonly BreadcrumbRegistration[]
}) {
  const [registrations, setRegistrations] = useState<readonly BreadcrumbRegistration[]>(() => {
    let list: readonly BreadcrumbRegistration[] = []
    for (const r of system) {
      const added = addRegistration(list, r)
      if (added.reason) console.error(`System breadcrumb ${added.reason} — ignoring it.`)
      else list = added.list
    }
    return list
  })
  // The list as of the last call, so two registrations in one tick both see each other (and a refusal is logged once).
  const latest = useRef(registrations)

  const register = useCallback((registration: BreadcrumbRegistration) => {
    const added = addRegistration(latest.current, registration)
    if (added.reason) {
      console.error(`Breadcrumb registration refused: ${added.reason}.`)
      return
    }
    latest.current = added.list
    setRegistrations(added.list)
  }, [])

  const value = useMemo(() => ({ registrations, register }), [registrations, register])
  return <BreadcrumbRegistryContext.Provider value={value}>{children}</BreadcrumbRegistryContext.Provider>
}
