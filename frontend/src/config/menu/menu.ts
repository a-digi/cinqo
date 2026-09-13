import { AppScopes } from '../security/scopes'

export interface MenuEntry {
  label: string
  // Omitted = not clickable, pure grouping label (toggles its own
  // children open/closed instead of navigating). Present = a real leaf.
  path?: string
  // OR-matched against the user's scopes; omitted = always visible
  // (once its parent chain has already survived filtering).
  scopes?: string[]
  children?: MenuEntry[]
}

export const menuEntries: MenuEntry[] = [
  { label: 'Home', path: '/' },
  {
    label: 'System',
    children: [
      {
        label: 'Security',
        children: [{ label: 'Scopes', path: '/admin/security/scopes', scopes: [AppScopes.SuperAdmin] }],
      },
      { label: 'Tools', path: '/admin/tools', scopes: [AppScopes.ToolManage] },
    ],
  },
]
