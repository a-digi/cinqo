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
import { CrawlLogsPage } from './CrawlLogsPage'
import './index.css'

const LIST_PATH = '/tools/browser/credentials'
const ADD_PATH = '/tools/browser/credentials/new'
const CRAWL_LOGS_PATH = '/tools/browser/crawl-logs'

window.__cinqoToolBridge.registerMenuEntry({
  label: 'Browser',
  children: [
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
      label: 'Crawl Logs',
      path: CRAWL_LOGS_PATH,
      // Reuses tool:browser:crawl — the same permission already
      // needed to run a crawl at all (step 17's own design).
      scopes: ['tool:browser:crawl'],
    },
  ],
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
  mount: (container) => mountReact(container, <ListPage />),
  unmount: unmountReact,
})

// No registerMenuEntry for this one — deliberately reachable only by
// clicking the list page's own "+" button (step 14's own design,
// unchanged by this rewrite).
window.__cinqoToolBridge.registerRoute({
  path: ADD_PATH,
  mount: (container) => mountReact(container, <AddPage />),
  unmount: unmountReact,
})

window.__cinqoToolBridge.registerRoute({
  path: CRAWL_LOGS_PATH,
  mount: (container) => mountReact(container, <CrawlLogsPage />),
  unmount: unmountReact,
})
