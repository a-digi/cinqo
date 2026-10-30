import { useEffect, useRef, useState } from 'react'
import { useToolRegistry } from '../../../config/tools/ToolRegistryContext'
import type { ToolDashboardWidget } from '../../../config/tools/toolBridge'
import { customFailedMessage, customMissingMessage } from '../widgetDisplay'
import type { CustomData } from '../widgetKinds'

function rendererFor(registry: Record<string, ToolDashboardWidget>, id: string): ToolDashboardWidget | undefined {
  return id in registry ? registry[id] : undefined
}

// A widget whose display a tool draws itself, with the renderer the tool's bundle
// registered for this widget id. The tool owns the card's body (an imperative
// mount/update/unmount, like a tool route); anything it throws is caught here so
// one tool's bug cannot take the dashboard down.
export function CustomWidget({ widgetId, data }: { widgetId: string; data: CustomData }) {
  const { dashboardWidgets } = useToolRegistry()
  const renderer = rendererFor(dashboardWidgets, widgetId)
  const containerRef = useRef<HTMLDivElement>(null)
  const latest = useRef(data)
  const mountedWith = useRef<typeof data | null>(null)
  const [failed, setFailed] = useState(false)

  // Keep the newest data where the mount effect can read it, without making the
  // renderer remount whenever data changes.
  useEffect(() => {
    latest.current = data
  })

  // Mount once per renderer (the bundle may register after the page has shown).
  useEffect(() => {
    const el = containerRef.current
    if (!el || !renderer) return
    try {
      renderer.mount(el, latest.current)
      mountedWith.current = latest.current
      setFailed(false)
    } catch (err) {
      console.error(`Dashboard widget "${widgetId}" failed to mount:`, err)
      setFailed(true)
      return
    }
    return () => {
      mountedWith.current = null
      try {
        renderer.unmount?.(el)
      } catch (err) {
        console.error(`Dashboard widget "${widgetId}" failed to unmount:`, err)
      }
      el.replaceChildren()
    }
  }, [renderer, widgetId])

  // Fresh data after the first draw: update in place if the tool can, else redraw.
  useEffect(() => {
    const el = containerRef.current
    if (!el || !renderer || mountedWith.current === null || mountedWith.current === data) return
    try {
      if (renderer.update) {
        renderer.update(el, data)
      } else {
        renderer.unmount?.(el)
        el.replaceChildren()
        renderer.mount(el, data)
      }
      mountedWith.current = data
      setFailed(false)
    } catch (err) {
      console.error(`Dashboard widget "${widgetId}" failed to update:`, err)
      setFailed(true)
    }
  }, [data, renderer, widgetId])

  if (!renderer) return <p className="text-sm text-gray-500">{customMissingMessage}</p>
  return (
    <>
      {failed && <p className="text-sm text-gray-500">{customFailedMessage}</p>}
      <div ref={containerRef} className={failed ? 'hidden' : 'h-full'} />
    </>
  )
}
