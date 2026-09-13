import { fetchTools } from '../../api/tools'
import { createToolBridge, type ToolBridgeCallbacks } from './toolBridge'

// Bootstrap: build the bridge and assign it to window BEFORE injecting
// any tool's script, so a tool's top-level code can call it
// synchronously on load. Only enabled tools with a frontend bundle get
// a <script> tag — a disabled tool's bundle is never injected into a
// normal session. See
// plan/ai/tools/step-07-frontend-bridge-and-menu-integration.md.
export async function loadTools(callbacks: ToolBridgeCallbacks): Promise<void> {
  window.__cinqoToolBridge = createToolBridge(callbacks)

  let tools
  try {
    tools = await fetchTools()
  } catch {
    return // no tools loaded this session — not fatal to the app itself
  }

  for (const tool of tools) {
    if (!tool.enabled || !tool.frontendBundleRelpath) continue
    const script = document.createElement('script')
    script.type = 'module'
    script.src = `/api/v1/tools/${encodeURIComponent(tool.slug)}/frontend-bundle?v=${encodeURIComponent(tool.version)}`
    document.body.appendChild(script)
  }
}

// The registry is append-only (no unregister) — matches coco-mda's own
// reasoning exactly: simpler than incrementally mutating live React
// state for every possible lifecycle change. Any install/enable/
// disable/delete just reloads the whole page instead.
export function reloadAfterToolChange(): void {
  setTimeout(() => window.location.reload(), 600)
}
