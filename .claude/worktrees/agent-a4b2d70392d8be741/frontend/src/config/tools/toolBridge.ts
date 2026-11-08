import { toolNavigate } from './toolNavigator'
import { openToolConversation } from './toolConversationOpener'

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
  // just documented.
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
  mount: (container: HTMLElement) => void
  unmount?: (container: HTMLElement) => void
}

export interface ToolBridge {
  registerMenuEntry(entry: ToolMenuEntry): void
  registerRoute(route: ToolRoute): void
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
