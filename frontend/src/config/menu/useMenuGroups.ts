import { useMemo } from 'react'
import { useAuth } from '../../Components/Auth/AuthContext'
import { AppScopes } from '../security/scopes'
import { useToolRegistry } from '../tools/ToolRegistryContext'
import { menuEntries, type MenuEntry } from './menu'
import { filterVisible } from './filterVisible'

// Simplified from coco-mda's real useMenuGroups: no i18n (cinqo has
// none). Tool-contributed entries (plan/ai/tools/step-07) are merged in
// as additional top-level entries, appended after the static app menu —
// matching coco-mda's own appGroups-then-pluginGroups ordering.
// filterVisible needs no changes at all to support this: it already
// operates generically on any MenuEntry[], so a tool's entries get the
// exact same recursive scope-hiding behavior for free. See
// plan/ai/frontend/frontend/step-06-nested-menu-system.md and
// plan/ai/tools/step-07-frontend-bridge-and-menu-integration.md.
export function useMenuGroups(): MenuEntry[] {
  const { scopes } = useAuth()
  const { menuEntries: toolMenuEntries } = useToolRegistry()
  const isSuperAdmin = scopes.includes(AppScopes.SuperAdmin)
  return useMemo(() => {
    const appGroups = filterVisible(menuEntries, isSuperAdmin, scopes)
    const toolGroups = filterVisible(toolMenuEntries, isSuperAdmin, scopes)
    return [...appGroups, ...toolGroups]
  }, [scopes, isSuperAdmin, toolMenuEntries])
}
