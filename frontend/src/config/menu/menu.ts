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
  // Raw SVG markup, sanitized by SidebarIcon before it ever touches
  // the DOM — deliberately the same shape/trust treatment as
  // ToolMenuEntry's own icon field (toolBridge.ts): one rendering path
  // for both core and plugin-supplied icons, not a trusted/untrusted
  // fork. Optional — an entry with none just renders without one. See
  // plan/ai/frontend/frontend/step-13-sidebar-icons-and-calculated-indentation.md.
  icon?: string
}

// Icons match icons.tsx's own established convention exactly
// (viewBox="0 0 20 20", stroke="currentColor", stroke-width
// 1.3-1.6) — plain, hand-drawn glyphs, not a brand/icon-set import.
const homeIcon =
  '<svg viewBox="0 0 20 20" fill="none" stroke="currentColor"><path d="M4 9l6-5 6 5" stroke-width="1.4" stroke-linecap="round" stroke-linejoin="round"/><path d="M5.5 8v6.5a1 1 0 0 0 1 1h7a1 1 0 0 0 1-1V8" stroke-width="1.4" stroke-linecap="round" stroke-linejoin="round"/></svg>'

const conversationsIcon =
  '<svg viewBox="0 0 20 20" fill="none" stroke="currentColor"><path d="M4 5.5h12a1 1 0 0 1 1 1v6a1 1 0 0 1-1 1H8l-3 3v-3H4a1 1 0 0 1-1-1v-6a1 1 0 0 1 1-1z" stroke-width="1.4" stroke-linejoin="round" stroke-linecap="round"/></svg>'

const toolsIcon =
  '<svg viewBox="0 0 20 20" fill="none" stroke="currentColor"><path d="M13.5 6.5a2.5 2.5 0 0 1-3.4 2.3l-4.6 4.6a1 1 0 0 1-1.4-1.4l4.6-4.6a2.5 2.5 0 1 1 4.8-2.3z" stroke-width="1.3" stroke-linejoin="round" stroke-linecap="round"/></svg>'

const platformsIcon =
  '<svg viewBox="0 0 20 20" fill="none" stroke="currentColor"><path d="M10 4l6 3-6 3-6-3 6-3z" stroke-width="1.3" stroke-linejoin="round"/><path d="M4 10l6 3 6-3" stroke-width="1.3" stroke-linejoin="round" stroke-linecap="round"/><path d="M4 13.5l6 3 6-3" stroke-width="1.3" stroke-linejoin="round" stroke-linecap="round"/></svg>'

const systemIcon =
  '<svg viewBox="0 0 20 20" fill="none" stroke="currentColor"><circle cx="10" cy="10" r="2.5" stroke-width="1.3"/><path d="M10 3v2M10 15v2M3 10h2M15 10h2M5.05 5.05l1.4 1.4M13.55 13.55l1.4 1.4M14.95 5.05l-1.4 1.4M6.45 13.55l-1.4 1.4" stroke-width="1.3" stroke-linecap="round"/></svg>'

const securityIcon =
  '<svg viewBox="0 0 20 20" fill="none" stroke="currentColor"><path d="M10 3l6 2.2v4.3c0 4-2.6 6.8-6 7.5-3.4-.7-6-3.5-6-7.5V5.2L10 3z" stroke-width="1.3" stroke-linejoin="round" stroke-linecap="round"/></svg>'

export const menuEntries: MenuEntry[] = [
  { label: 'Home', path: '/', icon: homeIcon },
  { label: 'Conversations', path: '/conversations', scopes: [AppScopes.ConversationUse], icon: conversationsIcon },
  { label: 'Tools', path: '/admin/tools', scopes: [AppScopes.ToolManage], icon: toolsIcon },
  { label: 'Platforms', path: '/admin/platforms', scopes: [AppScopes.PlatformRead], icon: platformsIcon },
  {
    label: 'System',
    icon: systemIcon,
    children: [
      {
        label: 'Security',
        icon: securityIcon,
        // Deliberately left without its own icon — a real,
        // deep (depth-3) leaf, both a natural fit (a single scope
        // list doesn't need its own glyph distinct from its
        // Security parent) and a live demonstration that an
        // icon-less entry still aligns correctly (see
        // SidebarMenuItem's own reserved icon-slot width).
        children: [{ label: 'Scopes', path: '/admin/security/scopes', scopes: [AppScopes.SuperAdmin] }],
      },
    ],
  },
]
