import { apiGet, apiPost } from './client'

// The update check (api/src/toolrepository/updatecheck): a background run that
// refreshes every repository, and the installed tools that have an update,
// always computed on the server from the stored catalogs. The backend already
// answers in camelCase.

export type CheckState = 'idle' | 'running' | 'done'

export interface CheckFailure {
  repositoryId: string
  name: string
  error: string
}

export interface UpdateCheck {
  state: CheckState
  runId: string
  trigger: 'launch' | 'manual' | ''
  startedAt: string
  finishedAt: string
  totalRepositories: number
  checkedRepositories: number
  failed: CheckFailure[]
  error: string
}

export interface ToolUpdate {
  slug: string
  name: string
  installedVersion: string
  latestVersion: string
  versionId: string
  repositoryId: string
  repositoryName: string
}

export interface ToolUpdatesResponse {
  check: UpdateCheck
  lastCheckedAt: string
  updates: ToolUpdate[]
}

const base = '/api/v1/tool-updates'

export async function fetchToolUpdates(): Promise<ToolUpdatesResponse> {
  const raw = await apiGet<{ message: ToolUpdatesResponse }>(base)
  return raw.message
}

// Starts a check in the background; an already running (or just finished) one is
// returned as it is, not an error.
export async function startUpdateCheck(): Promise<ToolUpdatesResponse> {
  const raw = await apiPost<{ message: ToolUpdatesResponse }>(`${base}/check`)
  return raw.message
}
