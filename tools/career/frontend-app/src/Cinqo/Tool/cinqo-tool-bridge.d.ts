// A local type-only copy of frontend/src/config/tools/toolBridge.ts's
// real interface — not imported, this tool's own build must stay
// fully self-contained. See main.tsx's own top comment and
// browser's own src/cinqo-tool-bridge.d.ts, copied verbatim.
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
  // Opens the main app's floating chat widget (if mounted) showing
  // this conversation, resuming its live turn watch automatically if
  // one is still running. See
  // plan/ai/tools/career/step-60-generate-with-ai-live-chat-window.md.
  openConversation(conversationId: string): void
}

declare global {
  interface Window {
    __cinqoToolBridge: CinqoToolBridge
  }
}

export {}
