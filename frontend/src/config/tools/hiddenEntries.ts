// Which of a tool's menu pages are hidden right now (plan/ai/tools/career/step-238-applications-menu-item-only-with-applications.md). Pure and free
// of React, so a tool author can copy it and it can be script-checked on its own.
import type { ToolMenuEntry } from './toolBridge'

// The addresses of the pages an entry (and everything below it) starts hidden: those with `hidden: true`.
export function initiallyHidden(entry: ToolMenuEntry): string[] {
  const out: string[] = []
  const walk = (e: ToolMenuEntry) => {
    if (e.hidden === true && typeof e.path === 'string' && e.path !== '') out.push(e.path)
    e.children?.forEach(walk)
  }
  walk(entry)
  return out
}

// The entries without the pages whose address is in `hidden`, at any depth. A group left with no page stays as an empty group: the menu's own
// scope filter drops a group with nothing in it, so the group disappears with its last page and comes back with the first one. Returns the same
// array when nothing is hidden (no new array, so nothing downstream recomputes).
export function applyHidden(entries: ToolMenuEntry[], hidden: ReadonlySet<string>): ToolMenuEntry[] {
  if (hidden.size === 0) return entries
  const walk = (list: ToolMenuEntry[]): ToolMenuEntry[] => {
    const out: ToolMenuEntry[] = []
    for (const e of list) {
      if (typeof e.path === 'string' && hidden.has(e.path)) continue
      out.push(e.children ? { ...e, children: walk(e.children) } : e)
    }
    return out
  }
  return walk(entries)
}
