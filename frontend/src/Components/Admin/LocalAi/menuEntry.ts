import { AppScopes } from '../../../config/security/scopes'
import type { MenuEntry } from '../../../config/menu/menu'

// The Local AI entry of the AI menu. Kept in its own file without any React code, so the menu configuration imports
// only this one small object. Nothing else of the module is imported outside this folder except route.tsx.
const icon =
  '<svg viewBox="0 0 20 20" fill="none" stroke="currentColor"><rect x="3.5" y="4" width="13" height="5" rx="1.3" stroke-width="1.3"/><rect x="3.5" y="11" width="13" height="5" rx="1.3" stroke-width="1.3"/><circle cx="6.5" cy="6.5" r="0.8" fill="currentColor" stroke="none"/><circle cx="6.5" cy="13.5" r="0.8" fill="currentColor" stroke="none"/><path d="M10 6.5h4M10 13.5h4" stroke-width="1.3" stroke-linecap="round"/></svg>'

export const localAiMenuEntry: MenuEntry = {
  label: 'Local Agents',
  path: '/admin/local-ai',
  scopes: [AppScopes.PlatformRead],
  icon,
}
