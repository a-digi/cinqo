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

interface CinqoOpenFileReaderOptions {
  tool: string
  source: string
  refs: string[]
  title?: string
  open?: string
  // "text" or "json" (the beautifier is offered and on); default: each file's name decides. An older host ignores it.
  format?: 'text' | 'json'

  back?: string
  backLabel?: string
}

interface CinqoToolBridge {
  registerMenuEntry(entry: CinqoToolMenuEntry): void
  registerRoute(route: CinqoToolRoute): void
  navigate(path: string): void
  // Opens Cinqo's log reader as a page of its own on one or several of this tool's files (a source declared in
  // manifest.json's "file_sources"), with a way back to the page the person came from. Optional: a host older than
  // this feature does not have it, and callers must check first.
  openFileReader?(options: CinqoOpenFileReaderOptions): void
  // The address openFileReader goes to (for a plain link), or null when an option is bad. Optional like openFileReader.
  fileReaderPath?(options: CinqoOpenFileReaderOptions): string | null
}

declare global {
  interface Window {
    __cinqoToolBridge: CinqoToolBridge
  }
}

export {}
