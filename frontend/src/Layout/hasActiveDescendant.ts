import type { MenuEntry } from '../config/menu/menu'

export function hasActiveDescendant(entry: MenuEntry, pathname: string): boolean {
  if (entry.path && (pathname === entry.path || pathname.startsWith(entry.path + '/'))) return true
  return entry.children?.some((child) => hasActiveDescendant(child, pathname)) ?? false
}
