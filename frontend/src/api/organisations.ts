import { apiGet, apiPost, apiPut } from './client'

// The operator's organisation API (api/src/organisation/handler). Answers use core's {success, message} wrapper; the fields are camelCase on the wire.

export type OrganisationStatus = 'active' | 'suspended'

export interface Organisation {
  id: string
  name: string
  status: OrganisationStatus
  createdAt: string
  updatedAt: string
  memberCount: number
  // Tools an administrator approved may work for this organisation without a signed-in person. Off until the operator switches it on.
  backgroundEnabled: boolean
}

export interface OrganisationMember {
  userId: string
  orgId: string
  createdAt: string
}

export async function fetchOrganisations(): Promise<Organisation[]> {
  const raw = await apiGet<{ message: { items: Organisation[] } }>('/api/v1/organisations')
  return raw.message.items
}

export async function createOrganisation(name: string): Promise<Organisation> {
  const raw = await apiPost<{ message: Organisation }>('/api/v1/organisations', { name })
  return raw.message
}

export async function updateOrganisation(id: string, input: { name: string; status: OrganisationStatus }): Promise<Organisation> {
  const raw = await apiPut<{ message: Organisation }>(`/api/v1/organisations/${encodeURIComponent(id)}`, input)
  return raw.message
}

export async function fetchMembers(orgId: string): Promise<OrganisationMember[]> {
  const raw = await apiGet<{ message: { items: OrganisationMember[] } }>(`/api/v1/organisations/${encodeURIComponent(orgId)}/members`)
  return raw.message.items
}

export async function assignUser(userId: string, orgId: string): Promise<void> {
  await apiPut(`/api/v1/users/${encodeURIComponent(userId)}/organisation`, { orgId })
}

export async function setOrganisationBackground(id: string, enabled: boolean): Promise<Organisation> {
  const raw = await apiPut<{ message: Organisation }>(`/api/v1/organisations/${encodeURIComponent(id)}/background`, { enabled })
  return raw.message
}

// What the operator sees of unattended tool use: the app-wide kill switch, the tools that ask to be used without a signed-in person (routes asked and approved,
// the approving is done on the Tools page), and the daily counters since `since` (a UTC day).
export interface BackgroundOverview {
  killSwitch: boolean
  days: number
  since: string
  tools: { slug: string; name: string; requested: number; approved: number }[]
  usage: { toolSlug: string; toolName: string; orgId: string; orgName: string; day: string; calls: number; denied: number }[]
}

export async function fetchBackgroundOverview(days: number): Promise<BackgroundOverview> {
  const raw = await apiGet<{ message: BackgroundOverview }>(`/api/v1/background/overview?days=${String(days)}`)
  return raw.message
}
