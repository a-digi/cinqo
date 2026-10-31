// A local type-only copy of frontend/src/config/tools/toolBridge.ts's
// real, already-shipped interface — not imported, since this tool's
// own build must stay fully self-contained (see main.tsx's own top
// comment and step 15's own "Deliberately does not import anything
// from frontend/src"). Kept in sync by hand; the actual runtime
// object is provided by the host app, this file only describes its
// shape to the compiler.
interface CinqoToolMenuEntry {
  label: string
  path?: string
  scopes?: string[]
  children?: CinqoToolMenuEntry[]
  icon?: string
}

interface CinqoToolRoute {
  path: string
  mount: (container: HTMLElement) => void
  unmount?: (container: HTMLElement) => void
}

interface CinqoToolBridge {
  registerMenuEntry(entry: CinqoToolMenuEntry): void
  registerRoute(route: CinqoToolRoute): void
  navigate(path: string): void
}

declare global {
  interface Window {
    __cinqoToolBridge: CinqoToolBridge
  }
}

export {}
