import { apiGet, apiPut } from './client'
import type { WidgetKind, WidgetState } from '../Components/Home/widgetKinds'

// The dashboard routes answer in camelCase already (api/src/dashboard/handler),
// so there is no *Raw mapping layer.
export interface WidgetDefinition {
  id: string
  provider: string
  title: string
  description: string
  kind: WidgetKind
  size: { w: number; h: number }
  refreshSeconds: number
  // This user's own choice; a hidden widget is still listed so Customize can show it again.
  hidden: boolean
}

export interface WidgetData {
  state: WidgetState
  kind: WidgetKind
  schemaVersion: number
  // Unvalidated on purpose: the renderer checks it with parseWidgetData.
  data?: unknown
  fetchedAt: string
}

export async function fetchWidgets(): Promise<WidgetDefinition[]> {
  const raw = await apiGet<{ message: WidgetDefinition[] }>('/api/v1/dashboard/widgets')
  return raw.message
}

export async function fetchWidgetData(id: string): Promise<WidgetData> {
  const raw = await apiGet<{ message: WidgetData }>(`/api/v1/dashboard/widgets/${encodeURIComponent(id)}/data`)
  return raw.message
}

// Saves the caller's own order and hidden flags. Ids the caller may not see are
// dropped by the backend; an empty list returns to the default arrangement.
export async function saveLayout(widgets: { id: string; hidden: boolean }[]): Promise<void> {
  await apiPut('/api/v1/dashboard/layout', { widgets })
}
