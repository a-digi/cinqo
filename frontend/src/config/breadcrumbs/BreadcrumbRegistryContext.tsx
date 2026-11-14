import { createContext, useCallback, useContext, useMemo, useRef, useState, type ReactNode } from 'react'
import { useLocation } from 'react-router-dom'
import { addRegistration, NO_LABELS, withLabel, type BreadcrumbRegistration, type LabelState } from './breadcrumbRules'
import { SYSTEM_BREADCRUMBS } from './systemBreadcrumbs'

export interface BreadcrumbRegistryValue {
  // Every trail registered so far: the system's first, then the tools' in the order they registered.
  registrations: readonly BreadcrumbRegistration[]
  // Adds one (a tool's, through the bridge). A bad or repeated path is refused with a console error and changes nothing.
  register: (registration: BreadcrumbRegistration) => void
  // The labels the current page set for items of its trail (step 7), tagged with the full address (path + query) they belong to.
  labelState: LabelState
  // A page of a tool puts its own words (the job's title) in place of an item's fixed label. A bad index or label is refused with a console
  // error. Belongs to the page shown now: a page change drops it.
  setLabel: (index: number, label: string) => void
}

const BreadcrumbRegistryContext = createContext<BreadcrumbRegistryValue>({
  registrations: [],
  register: () => undefined,
  labelState: NO_LABELS,
  setLabel: () => undefined,
})

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

  const { pathname, search } = useLocation()
  const [labelState, setLabelState] = useState<LabelState>(NO_LABELS)
  // The address as of the last render, so a label set by the page that just mounted is tagged with its own address.
  const addressRef = useRef(pathname + search)
  addressRef.current = pathname + search
  const setLabel = useCallback((index: number, label: string) => {
    setLabelState((prev) => {
      const next = withLabel(prev, addressRef.current, index, label)
      if (next.reason) console.error(`Breadcrumb label refused: ${next.reason}.`)
      return next.state
    })
  }, [])

  const value = useMemo(() => ({ registrations, register, labelState, setLabel }), [registrations, register, labelState, setLabel])
  return <BreadcrumbRegistryContext.Provider value={value}>{children}</BreadcrumbRegistryContext.Provider>
}
