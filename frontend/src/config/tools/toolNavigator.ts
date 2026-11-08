// Module-level indirection binding the bridge's navigate() to React
// Router's own navigate function while a tool route is mounted —
// avoids the bridge itself importing react-router-dom, keeping it
// dependency-free. Same pattern as coco-mda's own pluginNavigator.ts.
// See plan/ai/tools/step-07-frontend-bridge-and-menu-integration.md.
let current: ((path: string) => void) | null = null

export function setToolNavigator(fn: ((path: string) => void) | null): void {
  current = fn
}

export function toolNavigate(path: string): void {
  current?.(path)
}
