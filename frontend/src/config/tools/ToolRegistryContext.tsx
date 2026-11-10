import { createContext, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { useAuth } from '../../Components/Auth/AuthContext'
import { pdfThumbnail } from '../../Components/Admin/Media/pdfThumbnail'
import { mountComments } from '../../Components/Comments/mountComments'
import { mountFileReader } from '../../Components/FileReader/mountFileReader'
import { mountReviews, mountStars } from '../../Components/Reviews/mountReviews'
import { applyHidden, initiallyHidden } from './hiddenEntries'
import { loadTools } from './loadTools'
import type { ToolDashboardWidget, ToolMenuEntry, ToolRoute } from './toolBridge'

export interface ToolRegistryValue {
  menuEntries: ToolMenuEntry[]
  routes: ToolRoute[]
  // Tool-drawn dashboard widgets by widget id; the first registration of an id wins.
  dashboardWidgets: Record<string, ToolDashboardWidget>
}

const ToolRegistryContext = createContext<ToolRegistryValue>({ menuEntries: [], routes: [], dashboardWidgets: {} })

// Context + hook co-located deliberately (same convention as every other
// *Context.tsx in this codebase) — costs this file Fast Refresh for the
// hook specifically, never a runtime issue.
// eslint-disable-next-line react-refresh/only-export-components
export function useToolRegistry(): ToolRegistryValue {
  return useContext(ToolRegistryContext)
}

// Holds the live registry a tool's own bundle populates via the
// bridge's registerMenuEntry/registerRoute calls. Loads tools only
// once the session is actually authenticated — GET /api/v1/tools
// requires a session, and this provider sits above /login too (see
// AuthenticatedProviders' own placement rule). See
// plan/ai/tools/step-07-frontend-bridge-and-menu-integration.md.
export function ToolRegistryProvider({ children }: { children: ReactNode }) {
  const { isAuthenticated } = useAuth()
  const [registeredEntries, setRegisteredEntries] = useState<ToolMenuEntry[]>([])
  // The pages a tool shows or hides, by address: true = hidden. A page that starts hidden (hidden: true on its entry) is put here when it is
  // registered, unless the tool already decided about it; setMenuEntryHidden always wins.
  const [hiddenState, setHiddenState] = useState<Record<string, boolean>>({})
  const [routes, setRoutes] = useState<ToolRoute[]>([])
  const [dashboardWidgets, setDashboardWidgets] = useState<Record<string, ToolDashboardWidget>>({})
  const loadedRef = useRef(false)

  useEffect(() => {
    if (!isAuthenticated || loadedRef.current) return
    loadedRef.current = true
    void loadTools({
      onRegisterMenuEntry: (entry) => {
        setRegisteredEntries((prev) => [...prev, entry])
        const starts = initiallyHidden(entry)
        if (starts.length > 0) {
          setHiddenState((prev) => {
            const next = { ...prev }
            for (const path of starts) if (!(path in next)) next[path] = true
            return next
          })
        }
      },
      onSetMenuEntryHidden: (path, hidden) => {
        setHiddenState((prev) => (prev[path] === hidden ? prev : { ...prev, [path]: hidden }))
      },
      onRegisterRoute: (route) => {
        setRoutes((prev) => [...prev, route])
      },
      onMountComments: mountComments,
      onMountReviews: mountReviews,
      onMountStars: mountStars,
      onMountFileReader: mountFileReader,
      onPdfThumbnail: pdfThumbnail,
      onRegisterDashboardWidget: (widget) => {
        setDashboardWidgets((prev) => {
          if (widget.id in prev) {
            console.error(`Dashboard widget "${widget.id}" is already registered — ignoring the second registration.`)
            return prev
          }
          return { ...prev, [widget.id]: widget }
        })
      },
    })
  }, [isAuthenticated])

  // The menu without the pages a tool hides; the same array while nothing is hidden.
  const menuEntries = useMemo(
    () => applyHidden(registeredEntries, new Set(Object.keys(hiddenState).filter((path) => hiddenState[path]))),
    [registeredEntries, hiddenState],
  )

  return <ToolRegistryContext.Provider value={{ menuEntries, routes, dashboardWidgets }}>{children}</ToolRegistryContext.Provider>
}
