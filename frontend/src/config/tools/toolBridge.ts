import { toolNavigate } from './toolNavigator'

// Deliberately dependency-free (no React import, no cinqo-internal
// types beyond plain interfaces) — a tool author can copy just this
// file and write their bundle against it without pulling in cinqo's
// own frontend toolchain. Smaller than coco-mda's real PluginBridge —
// no registerDashboardWidget/registerPageAction/registerSettingsPage/
// getLanguage/isSidebarCollapsed (cinqo has none of dashboard widgets,
// page-action slots, i18n, or a collapsible sidebar today). See
// plan/ai/tools/step-07-frontend-bridge-and-menu-integration.md.
export interface ToolMenuEntry {
  label: string
  path?: string
  scopes?: string[]
  children?: ToolMenuEntry[]
}

export interface ToolRoute {
  // Must start with "/tools/" — enforced at registration time, not
  // just documented.
  path: string
  mount: (container: HTMLElement) => void
  unmount?: (container: HTMLElement) => void
}

export interface ToolBridge {
  registerMenuEntry(entry: ToolMenuEntry): void
  registerRoute(route: ToolRoute): void
  navigate(path: string): void
}

export interface ToolBridgeCallbacks {
  onRegisterMenuEntry: (entry: ToolMenuEntry) => void
  onRegisterRoute: (route: ToolRoute) => void
}

export function createToolBridge(callbacks: ToolBridgeCallbacks): ToolBridge {
  return {
    registerMenuEntry(entry) {
      callbacks.onRegisterMenuEntry(entry)
    },
    registerRoute(route) {
      if (!route.path.startsWith('/tools/')) {
        console.error(`Tool route "${route.path}" must start with "/tools/" — ignoring registration.`)
        return
      }
      callbacks.onRegisterRoute(route)
    },
    navigate(path) {
      toolNavigate(path)
    },
  }
}

declare global {
  interface Window {
    __cinqoToolBridge?: ToolBridge
  }
}
