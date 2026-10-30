// main.tsx is the Browser tool's own frontend entry point — a
// human-facing page for managing stored login credentials. As of
// step 15, authored in React + Tailwind and built to a single
// self-contained file, tools/browser/frontend/bundle.js — "React
// only for the build": the runtime contract with the host app
// (window.__cinqoToolBridge, ToolRoute.mount/unmount, one static JS
// file) is unchanged from the vanilla-JS version steps 10/12-14 built.
// See plan/ai/tools/browser/step-15-react-tailwind-frontend-rewrite.md.
import { createRoot, type Root } from 'react-dom/client'
import { ListPage } from './ListPage'
import { AddPage } from './AddPage'
import { ChromeGate } from './ChromeRequired'
import { BrowserEnginePage } from './BrowserEnginePage'
import { CrawlLogsPage } from './CrawlLogsPage'
import { DebugPage } from './DebugPage'
import { DomainsPage } from './DomainsPage'
import { fetchChromeStatus } from './api'
import './index.css'

const LIST_PATH = '/tools/browser/credentials'
const ADD_PATH = '/tools/browser/credentials/new'
const CRAWL_LOGS_PATH = '/tools/browser/crawl-logs'
const DEBUG_PATH = '/tools/browser/debug'
// "Browser & Engine" replaced the separate Chromium and Engine pages. The two old
// paths stay registered (bookmarks, and the link in the "Chrome is required"
// notice) and show the same page.
const BROWSER_ENGINE_PATH = '/tools/browser/browser-engine'
const ENGINE_PATH = '/tools/browser/engine'
const CHROMIUM_PATH = '/tools/browser/chromium'
const DOMAINS_PATH = '/tools/browser/domains'

// The gate (step 53): the other pages only make sense with a Chrome or
// Chromium installed. A tool's menu can only be appended to, so the decision is
// made once, here, when the bundle loads — after installing Chrome the page
// needs a reload. Fail open: if the status cannot be read, assume Chrome is
// there (the Browser & Engine page shows the error) rather than hide the tool.
const chromeReady: Promise<boolean> = fetchChromeStatus()
  .then((status) => status.installed)
  .catch(() => true)

const browserEngineEntry = {
  label: 'Browser & Engine',
  path: BROWSER_ENGINE_PATH,
  // Either existing scope: a user who has only the Login scope still
  // needs the one page that explains why nothing else works without
  // Chrome (menu visibility is "any of" — filterVisible.ts).
  scopes: ['tool:browser:login', 'tool:browser:crawl'],
}

// Pages shown only when Chrome is installed.
const chromeDependentEntries = [
  {
    label: 'Login',
    path: LIST_PATH,
    // Reuses the existing tool:browser:login scope — the same
    // permission that already gates writing a credential via the
    // underlying route, deliberately not a separate scope: a user
    // who can't submit a credential shouldn't see a page whose
    // only purpose is submitting one.
    scopes: ['tool:browser:login'],
  },
  {
    label: 'Domains',
    path: DOMAINS_PATH,
    // Step 96 — sites that asked for a check, and clearing them. Same scope as
    // the routes it calls (domains, domains/clear, crawl-cancel).
    scopes: ['tool:browser:crawl'],
  },
  {
    // Debug and Crawl Logs are one group: the Debug page controls the logging that
    // fills Crawl Logs, so they sit together under a parent of the same name.
    label: 'Debug',
    children: [
      {
        label: 'Debug',
        path: DEBUG_PATH,
        // Reuses tool:browser:crawl (step 22's own design) — same
        // permission tier as Crawl Logs, which this page's own Debug
        // toggle controls the logging behind.
        scopes: ['tool:browser:crawl'],
      },
      {
        label: 'Crawl Logs',
        path: CRAWL_LOGS_PATH,
        // Reuses tool:browser:crawl — the same permission already
        // needed to run a crawl at all (step 17's own design).
        scopes: ['tool:browser:crawl'],
      },
    ],
  },
]

void chromeReady.then((installed) => {
  window.__cinqoToolBridge.registerMenuEntry({
    label: 'Browser',
    // A globe/web glyph — the sidebar's own sanitizer allowlist (circle,
    // ellipse, line, plus viewBox/stroke-width) is deliberately narrow,
    // so this is built only from those primitives. See
    // plan/ai/tools/step-15-menu-icons-for-browser-and-career.md.
    icon: '<svg viewBox="0 0 20 20" fill="none" stroke="currentColor"><circle cx="10" cy="10" r="7" stroke-width="1.3"/><ellipse cx="10" cy="10" rx="3" ry="7" stroke-width="1.3"/><line x1="3" y1="10" x2="17" y2="10" stroke-width="1.3"/></svg>',
    children: installed ? [browserEngineEntry, ...chromeDependentEntries] : [browserEngineEntry],
  })
})

// Each container gets its own React root, stashed on the DOM node
// itself so the matching unmount call (a *different* function
// reference, called later by ToolRouteOutlet) can find and tear it
// down. mount/unmount both operate on whatever container the host
// app hands in — no assumption about there being only ever one.
const roots = new WeakMap<HTMLElement, Root>()

function mountReact(container: HTMLElement, node: React.ReactNode) {
  const root = createRoot(container)
  roots.set(container, root)
  root.render(node)
}

function unmountReact(container: HTMLElement) {
  roots.get(container)?.unmount()
  roots.delete(container)
}

window.__cinqoToolBridge.registerRoute({
  path: LIST_PATH,
  mount: (container) => {
    mountReact(
      container,
      <ChromeGate ready={chromeReady}>
        <ListPage />
      </ChromeGate>,
    )
  },
  unmount: unmountReact,
})

// No registerMenuEntry for this one — deliberately reachable only by
// clicking the list page's own "+" button (step 14's own design,
// unchanged by this rewrite).
window.__cinqoToolBridge.registerRoute({
  path: ADD_PATH,
  mount: (container) => {
    mountReact(
      container,
      <ChromeGate ready={chromeReady}>
        <AddPage />
      </ChromeGate>,
    )
  },
  unmount: unmountReact,
})

window.__cinqoToolBridge.registerRoute({
  path: CRAWL_LOGS_PATH,
  mount: (container) => {
    mountReact(
      container,
      <ChromeGate ready={chromeReady}>
        <CrawlLogsPage />
      </ChromeGate>,
    )
  },
  unmount: unmountReact,
})

// The merged page is reachable at its own path and at the two old ones, and is
// never gated: it is where the tool explains that Chrome is required.
for (const path of [BROWSER_ENGINE_PATH, CHROMIUM_PATH, ENGINE_PATH]) {
  window.__cinqoToolBridge.registerRoute({
    path,
    mount: (container) => {
      mountReact(container, <BrowserEnginePage />)
    },
    unmount: unmountReact,
  })
}

window.__cinqoToolBridge.registerRoute({
  path: DOMAINS_PATH,
  mount: (container) => {
    mountReact(
      container,
      <ChromeGate ready={chromeReady}>
        <DomainsPage />
      </ChromeGate>,
    )
  },
  unmount: unmountReact,
})

window.__cinqoToolBridge.registerRoute({
  path: DEBUG_PATH,
  mount: (container) => {
    mountReact(
      container,
      <ChromeGate ready={chromeReady}>
        <DebugPage />
      </ChromeGate>,
    )
  },
  unmount: unmountReact,
})
