import { toolNavigate } from './toolNavigator'
import { openToolConversation } from './toolConversationOpener'
import { isMediaId } from './mediaThumbnailRules'
import { isValidToolRoutePath } from './matchToolRoute'

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
  // SVG markup only — sanitized by SidebarIcon (frontend/src/Layout/SidebarIcon.tsx)
  // before it ever reaches the DOM, since this string comes from a
  // tool's own (untrusted) bundle. Only a small allowlist of
  // shape-drawing elements/attributes survives sanitization (svg,
  // path, g, circle, rect, line, polyline, polygon, ellipse, defs;
  // d/viewBox/fill/stroke/stroke-width/stroke-linecap/stroke-linejoin/
  // cx/cy/r/rx/ry/x/y/x1/y1/x2/y2/points/width/height) — no
  // <script>/<foreignObject>/<use>/<image>, no on* attributes, no
  // href/xlink:href, no style attribute. The root <svg>'s own
  // width/height are always discarded and replaced with a fixed
  // display size, so an oversized icon can't break the sidebar's own
  // row layout. A string longer than 2000 characters is rejected
  // outright, before parsing. See
  // plan/ai/frontend/frontend/step-13-sidebar-icons-and-calculated-indentation.md.
  icon?: string
}

export interface ToolRoute {
  // Must start with "/tools/" — enforced at registration time, not
  // just documented. Either exact ("/tools/career/skills") or a pattern
  // with named segments ("/tools/career/skills/:personaId"): a ":name"
  // segment matches one non-empty segment of the address, and its
  // (URL-decoded) text reaches mount as params.name. An exact path wins
  // over a pattern. See matchToolRoute.ts.
  path: string
  // A tool's own bundle.js is installed at runtime and never passes
  // through this app's Tailwind build — it can never use a Tailwind
  // utility class, no matter how it's wired. For visual consistency
  // with the rest of the app, reference these CSS custom properties
  // (always present on :root, defined in frontend/src/index.css) from
  // plain inline styles or a <style> tag, with a literal fallback on
  // every one so the page still renders sensibly if they're ever
  // missing:
  //   --cinqo-tool-text          body text color
  //   --cinqo-tool-text-muted    secondary/helper text color
  //   --cinqo-tool-border        border color
  //   --cinqo-tool-surface       subtle background (e.g. table header)
  //   --cinqo-tool-primary       primary/CTA background color
  //   --cinqo-tool-primary-text  text color on top of --cinqo-tool-primary
  //   --cinqo-tool-danger        error/destructive text or accent color
  //   --cinqo-tool-radius        standard corner radius
  //   --cinqo-tool-font          the app's own font stack
  //   --cinqo-tool-input-bg      form input background (white)
  //   --cinqo-tool-primary-hover hover background for a primary/CTA button
  //   --cinqo-tool-danger-hover  hover background for a danger/destructive button
  //   --cinqo-tool-shadow        subtle card/elevation shadow
  //   --cinqo-tool-icon-hover    hover background for a small icon-only button
  // e.g. `container.style.border = '1px solid var(--cinqo-tool-border, #e5e7eb)'`.
  // See plan/ai/tools/browser/step-12-menu-grouping-and-tool-styling-contract.md,
  // step-13-credential-page-visual-refresh.md, and
  // step-14-separate-add-credential-page.md.
  // params holds the text of each ":name" segment of the path ({} for an
  // exact path); a mount that takes only the container keeps working.
  mount: (container: HTMLElement, params: Record<string, string>) => void
  unmount?: (container: HTMLElement) => void
}

// A tool's own display for one of its Home dashboard widgets, for a widget the
// tool declared with kind "custom" (manifest widgets[]). Core fetches and
// validates the widget's data exactly as for any other widget and hands it to
// the tool, which draws it into the card's body. The same rules as ToolRoute's
// mount apply: no Tailwind classes, only inline styles and the --cinqo-tool-*
// CSS variables listed there.
export interface ToolDashboardWidget {
  // The widget's id: "<tool slug>.<widget key>", e.g. "career.new_jobs".
  id: string
  // Draw into container. data is the JSON object the tool's own data route
  // returned (a plain object, at most 8 levels deep).
  mount: (container: HTMLElement, data: Record<string, unknown>) => void
  // Called when fresh data arrives. Omit it and core will unmount and mount
  // again instead.
  update?: (container: HTMLElement, data: Record<string, unknown>) => void
  // Clean up (timers, listeners). Core also empties the container afterwards.
  unmount?: (container: HTMLElement) => void
}

// Where to draw the comments panel and which entity it is about.
export interface ToolCommentsOptions {
  // The tool's own slug, e.g. "career" (comments are kept per owner).
  owner: string
  // An entity type the tool declared in manifest.json's "comments", e.g. "job".
  entityType: string
  // The entity's UUID.
  entityId: string
  // Heading of the panel (default "Comments").
  title?: string
}

// Where to draw the file reader and which file it shows.
export interface ToolFileReaderOptions {
  // The tool's own slug, e.g. "browser".
  tool: string
  // A file source the tool declared in manifest.json's "file_sources", e.g. "crawl-logs".
  source: string
  // The file, as a path relative to that source's folder in the tool's own data, e.g. "crawl/2026-10-31/run-1.log".
  // Never an absolute path: core refuses anything that would leave the folder.
  ref: string
  // The name in the window's title bar (default: the ref).
  title?: string
  // How many lines are shown at once: 50, 100, 200 or 500 (default 200).
  initialWindow?: number
}

export interface ToolBridge {
  registerMenuEntry(entry: ToolMenuEntry): void
  registerRoute(route: ToolRoute): void
  // Registers the display for a "custom" dashboard widget. The first
  // registration of an id wins; a malformed id or a missing mount is refused
  // with a console error. A throwing mount/update shows a "couldn't be
  // displayed" note in that card and nothing else on the page is affected.
  registerDashboardWidget(widget: ToolDashboardWidget): void
  // Draws Cinqo's comments panel for one of the tool's entities into container
  // and returns a function that removes it (call it from your route's unmount).
  // The panel handles listing, adding, editing, deleting and filtering; who may do
  // what is decided by the server from the scopes in manifest.json's "comments".
  // Mounting into a container that already has a panel replaces it. A bad option is
  // refused with a console error and returns a function that does nothing.
  mountComments(container: HTMLElement, options: ToolCommentsOptions): () => void
  // Draws Cinqo's terminal-style file reader for one text file of the tool's own data into container and
  // returns a function that removes it (call it from your route's unmount). It reads big files a window of up to
  // 500 lines at a time, with search, never loading a file whole. Who may read is decided by the server from the
  // scope in manifest.json's "file_sources". A bad option is refused with a console error and returns a function
  // that does nothing.
  mountFileReader(container: HTMLElement, options: ToolFileReaderOptions): () => void
  // The first page of a PDF in Media as a JPEG data URL, rendered by Cinqo (a tool bundle is a
  // single file and cannot ship pdf.js and its worker), or null when it cannot be previewed
  // (not a PDF, deleted, unreadable, an id that is not a Media file id). The file is fetched with
  // the signed-in user's own session, exactly as the Media page and a download link do; only an
  // id is accepted, never a URL. version is anything that changes when the file does (a
  // document's updatedAt): it only keeps a replaced file from being shown stale. Renders run one
  // at a time and are cached per id and version. Call it when the preview scrolls into view.
  pdfThumbnail(mediaFileId: string, version?: string): Promise<string | null>
  navigate(path: string): void
  // Opens the floating chat widget (GlobalChatWidget), if currently
  // mounted, showing this conversation — resuming its live turn watch
  // automatically if one is still running. A conversationId the
  // caller isn't the owner of behaves exactly like a user navigating
  // to one they don't own today: the widget's own existing error
  // handling shows "Failed to load conversation," nothing more. See
  // plan/ai/tools/career/step-60-generate-with-ai-live-chat-window.md.
  openConversation(conversationId: string): void
}

export interface ToolBridgeCallbacks {
  onRegisterMenuEntry: (entry: ToolMenuEntry) => void
  onRegisterRoute: (route: ToolRoute) => void
  onRegisterDashboardWidget: (widget: ToolDashboardWidget) => void
  // Supplied by the app, which owns the React side of the comments panel; this file
  // stays free of React so a tool author can copy it on its own.
  onMountComments: (container: HTMLElement, options: ToolCommentsOptions) => () => void
  // Supplied by the app, which owns the React side of the file reader.
  onMountFileReader: (container: HTMLElement, options: ToolFileReaderOptions) => () => void
  // Supplied by the app, which owns the PDF renderer (pdf.js is only loaded when first needed).
  onPdfThumbnail: (mediaFileId: string, version?: string) => Promise<string | null>
}

const widgetIdPattern = /^[a-z][a-z0-9_]*\.[a-z][a-z0-9_]*$/

export function createToolBridge(callbacks: ToolBridgeCallbacks): ToolBridge {
  return {
    registerMenuEntry(entry) {
      callbacks.onRegisterMenuEntry(entry)
    },
    registerRoute(route) {
      if (!isValidToolRoutePath(route.path)) {
        console.error(
          `Tool route "${route.path}" must start with "/tools/", have no empty segment and use each ":name" once — ignoring registration.`,
        )
        return
      }
      callbacks.onRegisterRoute(route)
    },
    registerDashboardWidget(widget) {
      if (typeof widget.id !== 'string' || !widgetIdPattern.test(widget.id)) {
        console.error(`Dashboard widget id ${JSON.stringify(widget.id)} must look like "<tool>.<key>" — ignoring registration.`)
        return
      }
      if (typeof widget.mount !== 'function') {
        console.error(`Dashboard widget "${widget.id}" must provide a mount function — ignoring registration.`)
        return
      }
      callbacks.onRegisterDashboardWidget(widget)
    },
    mountComments(container, options) {
      return callbacks.onMountComments(container, options)
    },
    mountFileReader(container, options) {
      return callbacks.onMountFileReader(container, options)
    },
    pdfThumbnail(mediaFileId, version) {
      // Anything that is not a Media file id gets no request at all.
      if (!isMediaId(mediaFileId)) return Promise.resolve(null)
      return callbacks.onPdfThumbnail(mediaFileId, typeof version === 'string' ? version : undefined)
    },
    navigate(path) {
      toolNavigate(path)
    },
    openConversation(conversationId) {
      openToolConversation(conversationId)
    },
  }
}

declare global {
  interface Window {
    __cinqoToolBridge?: ToolBridge
  }
}
