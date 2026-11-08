import { ApiError, apiDelete, apiGet, apiPost } from './client'
import type { Publisher, ToolAuthor, ToolMetadata } from './toolMeta'

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
  // Who maintains it, as its file states; null when the file states nothing (the older format).
  publisher: Publisher | null
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
  authors: ToolAuthor[]
  license: string
}

// One tool a version needs, as the repository file lists it; empty bounds mean none.
export interface RepositoryRequirement {
  id: string
  minVersion: string
  maxVersion: string
}

export interface RepositoryTool {
  id: string
  name: string
  description: string
  status: ToolStatus
  reason: string
  // Who made the current version, as the repository states it; null when it states nothing.
  metadata: ToolMetadata | null
  installedVersion: string
  currentVersion: string
  currentVersionId: string
  // What the current version needs, in the order the file lists it (empty from a core that predates it).
  requires: RepositoryRequirement[]
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

// The ordered plan of an install or update together with the tools it needs (plan dependency-install, steps 4 and 6).
export type PlanAction = 'install' | 'update' | 'enable'
export type PlanMark = 'other_repository' | 'local_network' | 'stale_repository' | 'restarts' | 'was_disabled'

export interface InstallPlanStep {
  toolId: string
  name: string
  action: PlanAction
  versionId: string
  version: string
  installedVersion: string
  repositoryId: string
  repositoryName: string
  sizeBytes: number
  authors: string[]
  license: string
  requiredBy: string[]
  marks: PlanMark[]
  dependents: string[]
  enableAfter: boolean
}

export interface InstallPlanProblem {
  toolId: string
  requiredBy: string[]
  kind: string
  message: string
}

export interface InstallPlan {
  ok: boolean
  steps: InstallPlanStep[]
  totalBytes: number
  problems: InstallPlanProblem[]
  searched: { id: string; name: string; stale: boolean; refreshedAt: string }[]
  // The steps as the install must be given them back to prove the person saw them.
  confirm: string[]
  // Other tools to update instead when an installed tool holds this one back (plan step 10).
  alternatives: InstallPlanAlternative[]
}

export interface InstallPlanAlternative {
  forToolId: string
  toolId: string
  name: string
  version: string
  versionId: string
  repositoryId: string
  repositoryName: string
}

export interface DoneStep {
  toolId: string
  action: string
  version: string
}

// What the install with dependencies answers: done, or why not (a changed plan comes back with the new plan; a failed step says what was and was not done).
export interface InstallRun {
  ok: boolean
  code: 'done' | 'plan_blocked' | 'plan_changed' | 'step_failed'
  message: string
  installed: DoneStep[]
  notDone: DoneStep[]
  plan?: InstallPlan
}

export async function fetchInstallPlan(id: string, toolId: string, versionId?: string): Promise<InstallPlan> {
  const raw = await apiPost<{ message: InstallPlan }>(
    `${base}/${encodeURIComponent(id)}/tools/${encodeURIComponent(toolId)}/install-plan`,
    versionId ? { versionId } : undefined,
  )
  return raw.message
}

// 409 (plan blocked or changed) and 422 (a step failed) carry an InstallRun, not a message: they are answers, not errors. Anything else is thrown.
export async function installWithDependencies(id: string, toolId: string, confirm: string[], versionId?: string): Promise<InstallRun> {
  try {
    const raw = await apiPost<{ message: InstallRun }>(`${base}/${encodeURIComponent(id)}/tools/${encodeURIComponent(toolId)}/install`, {
      versionId,
      withDependencies: true,
      confirm,
    })
    return raw.message
  } catch (err) {
    if (err instanceof ApiError && (err.status === 409 || err.status === 422)) {
      const body = err.body as { message?: unknown } | undefined
      if (body && typeof body.message === 'object' && body.message !== null && 'code' in body.message) {
        return body.message as InstallRun
      }
    }
    throw err
  }
}
