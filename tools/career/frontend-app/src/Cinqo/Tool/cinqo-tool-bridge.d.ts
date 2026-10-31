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
  // Exact ("/tools/career/skills") or a pattern with named segments ("/tools/career/skills/:personaId"): a
  // ":name" segment matches one non-empty segment of the address and arrives in mount's params.
  path: string
  // params holds the text of each ":name" segment ({} for an exact path); a mount that takes only the
  // container keeps working. A host older than pattern routes ignores a pattern path (nothing matches it) and
  // passes no params.
  mount: (container: HTMLElement, params: Record<string, string>) => void
  unmount?: (container: HTMLElement) => void
}

interface CinqoCommentsOptions {
  owner: string
  entityType: string
  entityId: string
  title?: string
}

interface CinqoFileReaderOptions {
  tool: string
  source: string
  ref: string
  title?: string
  initialWindow?: 50 | 100 | 200 | 500
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
  // Draws Cinqo's comments panel for one of this tool's entities into container
  // (an entity type declared in manifest.json's "comments") and returns a function
  // that removes it. Optional: a host older than the comments feature does not
  // have it, and callers must check before using it.
  mountComments?(container: HTMLElement, options: CinqoCommentsOptions): () => void
  // Draws Cinqo's terminal-style file reader for one text file of this tool's own data (a source
  // declared in manifest.json's "file_sources") into container and returns a function that removes
  // it. Optional: a host older than the file reader does not have it, and callers must check first.
  mountFileReader?(container: HTMLElement, options: CinqoFileReaderOptions): () => void
  // The first page of a PDF in Media as a JPEG data URL, rendered by Cinqo, or null when it cannot
  // be previewed. Optional: a host older than this feature does not have it, and callers must check
  // before using it. version (e.g. a document's updatedAt) only separates cached renders.
  pdfThumbnail?(mediaFileId: string, version?: string): Promise<string | null>
}

declare global {
  interface Window {
    __cinqoToolBridge: CinqoToolBridge
  }
}

export {}
