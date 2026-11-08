import { useEffect, useRef } from 'react'
import { useLocation, useNavigate } from 'react-router-dom'
import { useToolRegistry } from '../../config/tools/ToolRegistryContext'
import { setToolNavigator } from '../../config/tools/toolNavigator'

// The one static SPA route hosting every tool route — a tool never
// gets a real React Router route of its own, matching coco-mda's own
// PluginRouteOutlet exactly. See
// plan/ai/tools/step-07-frontend-bridge-and-menu-integration.md.
export function ToolRouteOutlet() {
  const location = useLocation()
  const navigate = useNavigate()
  const { routes } = useToolRegistry()
  const containerRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    setToolNavigator((path) => {
      void navigate(path)
    })
    return () => {
      setToolNavigator(null)
    }
  }, [navigate])

  useEffect(() => {
    const route = routes.find((r) => r.path === location.pathname)
    const container = containerRef.current
    if (!route || !container) return

    route.mount(container)
    return () => {
      route.unmount?.(container)
    }
  }, [location.pathname, routes])

  return <div ref={containerRef} />
}
