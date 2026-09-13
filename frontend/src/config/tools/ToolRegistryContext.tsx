import { createContext, useContext, useEffect, useRef, useState, type ReactNode } from 'react'
import { useAuth } from '../../Components/Auth/AuthContext'
import { loadTools } from './loadTools'
import type { ToolMenuEntry, ToolRoute } from './toolBridge'

export interface ToolRegistryValue {
  menuEntries: ToolMenuEntry[]
  routes: ToolRoute[]
}

const ToolRegistryContext = createContext<ToolRegistryValue>({ menuEntries: [], routes: [] })

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
  const [menuEntries, setMenuEntries] = useState<ToolMenuEntry[]>([])
  const [routes, setRoutes] = useState<ToolRoute[]>([])
  const loadedRef = useRef(false)

  useEffect(() => {
    if (!isAuthenticated || loadedRef.current) return
    loadedRef.current = true
    void loadTools({
      onRegisterMenuEntry: (entry) => setMenuEntries((prev) => [...prev, entry]),
      onRegisterRoute: (route) => setRoutes((prev) => [...prev, route]),
    })
  }, [isAuthenticated])

  return <ToolRegistryContext.Provider value={{ menuEntries, routes }}>{children}</ToolRegistryContext.Provider>
}
