// The trails the breadcrumb bar shows on the Browser pages (plan/ai/frontend/step-270-breadcrumb.md, "a trail on all pages"). The words are the
// menu's own (main.tsx); a menu group has no page, so it is plain text. Every route main.tsx registers has one (a test checks it).

type Bridge = Window['__cinqoToolBridge']
type Trail = Parameters<NonNullable<Bridge['registerBreadcrumbs']>>[0]

export const BROWSER_ENGINE_PATH = '/tools/browser/browser-engine'
// The earlier addresses of the same page.
export const CHROMIUM_PATH = '/tools/browser/chromium'
export const ENGINE_PATH = '/tools/browser/engine'
export const DOMAINS_PATH = '/tools/browser/domains'
export const DEBUG_PATH = '/tools/browser/debug'
export const CRAWL_LOGS_PATH = '/tools/browser/crawl-logs'

export const BROWSER_BREADCRUMBS: readonly Trail[] = [
  { path: BROWSER_ENGINE_PATH, items: [{ label: 'Browser & Engine' }] },
  { path: CHROMIUM_PATH, items: [{ label: 'Browser & Engine' }] },
  { path: ENGINE_PATH, items: [{ label: 'Browser & Engine' }] },
  { path: DOMAINS_PATH, items: [{ label: 'Challenges' }, { label: 'Domains' }] },
  { path: DEBUG_PATH, items: [{ label: 'Debug' }] },
  { path: CRAWL_LOGS_PATH, items: [{ label: 'Debug' }, { label: 'Crawl Logs' }] },
]

// Hands the trails to the host. A host older than plan 270 has no registerBreadcrumbs: nothing is registered and nothing breaks.
export function registerBrowserBreadcrumbs(bridge: { registerBreadcrumbs?: Bridge['registerBreadcrumbs'] }): void {
  if (typeof bridge.registerBreadcrumbs !== 'function') return
  for (const trail of BROWSER_BREADCRUMBS) bridge.registerBreadcrumbs(trail)
}
