// Pure rules for the Home dashboard's Customize view: reordering, hiding, and
// what gets saved. No React, so each can be script-checked on its own.

export interface LayoutItem {
  id: string
  hidden: boolean
}

export function moveItem<T extends LayoutItem>(items: T[], id: string, direction: -1 | 1): T[] {
  const from = items.findIndex((i) => i.id === id)
  const to = from + direction
  if (from < 0 || to < 0 || to >= items.length) return items
  const next = [...items]
  ;[next[from], next[to]] = [next[to], next[from]]
  return next
}

export function toggleHidden<T extends LayoutItem>(items: T[], id: string): T[] {
  return items.map((i) => (i.id === id ? { ...i, hidden: !i.hidden } : i))
}

export function visibleItems<T extends LayoutItem>(items: T[]): T[] {
  return items.filter((i) => !i.hidden)
}

export function toPayload(items: LayoutItem[]): LayoutItem[] {
  return items.map(({ id, hidden }) => ({ id, hidden }))
}

// True when the arrangement (order and hidden flags) is the same.
export function sameLayout(a: LayoutItem[], b: LayoutItem[]): boolean {
  return a.length === b.length && a.every((item, i) => item.id === b[i].id && item.hidden === b[i].hidden)
}
