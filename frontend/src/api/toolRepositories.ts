import { apiDelete, apiGet, apiPost } from './client'

// The backend already answers in camelCase for these routes, so there is no
// *Raw mapping layer — see api/src/toolrepository.

export type ToolStatus = 'not_installed' | 'up_to_date' | 'update_available' | 'newer_than_repository' | 'unavailable'

export interface RepositoryCounts {
  tools: number
  installed: number
  upToDate: number
  updatable: number
  notInstalled: number
}

export interface ToolRepository {
  id: string
  url: string
  name: string
  description: string
  isLocal: boolean
  // Cinqo's own built-in repository (address from config.json): always
  // present, cannot be removed.
  isNative: boolean
  lastFetchedAt: string
  lastStatus: 'never' | 'ok' | 'error'
  lastError: string
  createdAt: string
  updatedAt: string
  counts: RepositoryCounts
}

export interface RepositoryToolVersion {
  versionId: string
  name: string
  description: string
  version: string
  releasedAt: string
  changelog: string
  minAppVersion: string
  available: boolean
  reason: string
}

export interface RepositoryTool {
  id: string
  name: string
  description: string
  status: ToolStatus
  reason: string
  installedVersion: string
  currentVersion: string
  currentVersionId: string
  versions: RepositoryToolVersion[]
}

export interface ToolRepositoryDetail extends ToolRepository {
  tools: RepositoryTool[]
}

const base = '/api/v1/tool-repositories'

export async function fetchRepositories(): Promise<ToolRepository[]> {
  const raw = await apiGet<{ message: ToolRepository[] }>(base)
  return raw.message
}

export async function fetchRepository(id: string): Promise<ToolRepositoryDetail> {
  const raw = await apiGet<{ message: ToolRepositoryDetail }>(`${base}/${encodeURIComponent(id)}`)
  return raw.message
}

// The backend fetches and validates the file before storing anything; a
// rejected file answers 400 with every problem in the message.
export async function addRepository(url: string): Promise<void> {
  await apiPost(base, { url })
}

export async function removeRepository(id: string): Promise<void> {
  await apiDelete(`${base}/${encodeURIComponent(id)}`)
}

// A failed refresh still answers 200; the repository then shows lastStatus
// "error" and keeps its last good catalog.
export async function refreshRepository(id: string): Promise<void> {
  await apiPost(`${base}/${encodeURIComponent(id)}/refresh`)
}

export async function installFromRepository(id: string, toolId: string, versionId?: string): Promise<void> {
  await apiPost(`${base}/${encodeURIComponent(id)}/tools/${encodeURIComponent(toolId)}/install`, versionId ? { versionId } : undefined)
}
