import { useMemo } from 'react'
import { useAuth } from '../../Components/Auth/AuthContext'
import { AppScopes } from '../security/scopes'
import { menuEntries, type MenuEntry } from './menu'
import { filterVisible } from './filterVisible'

// Simplified from coco-mda's real useMenuGroups: no i18n (cinqo has
// none), no plugin-entry merging (cinqo has no plugin system yet). See
// plan/ai/frontend/frontend/step-06-nested-menu-system.md.
export function useMenuGroups(): MenuEntry[] {
  const { scopes } = useAuth()
  const isSuperAdmin = scopes.includes(AppScopes.SuperAdmin)
  return useMemo(() => filterVisible(menuEntries, isSuperAdmin, scopes), [scopes, isSuperAdmin])
}
