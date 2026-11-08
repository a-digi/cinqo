import { ApiError, apiDelete, apiGet, apiPost, apiUpload } from './client'
import type { ToolMetadata } from './toolMeta'

// Same snake_case-to-camelCase *Raw mapping convention as api/auth.ts,
// api/security.ts. install_path/pid are never sent by the backend
// (json:"-" on the Go entity) — nothing server-internal to map here.

// Another tool this one declared it cannot work without (plan 217); minVersion is empty when any version will do, maxVersion when there is no
// upper bound (the highest version it works with, inclusive).
export interface RequiredTool {
  slug: string
  minVersion: string
  maxVersion: string
}

export interface Tool {
  id: string
  slug: string
  name: string
  version: string
  kind: 'frontend_only' | 'backend_only' | 'frontend_and_backend'
  enabled: boolean
  status: 'installed' | 'starting' | 'running' | 'stopped' | 'error'
  frontendBundleRelpath: string
  minAppVersion: string
  maxAppVersion: string
  createdAt: string
  updatedAt: string
  // Names only, not description/inputSchema/requiredScope — the Tools
  // page's own pills just need a label. See ToolsListPage.tsx.
  mcpTools: string[]
  // Who made it, as its own package declared; null for a tool installed before that was recorded.
  metadata: ToolMetadata | null
  // The tools this one needs, in the order it declared them.
  requiredTools: RequiredTool[]
}

interface ToolRaw {
  id: string
  slug: string
  name: string
  version: string
  kind: Tool['kind']
  enabled: boolean
  status: Tool['status']
  frontend_bundle_relpath: string
  min_app_version: string
  max_app_version: string
  created_at: string
  updated_at: string
  mcp_tools: string[]
  metadata?: ToolMetadata | null
  // Absent from a core that predates it: read as "needs nothing".
  required_tools?: { slug: string; min_version?: string; max_version?: string }[] | null
}

function fromRaw(raw: ToolRaw): Tool {
  return {
    id: raw.id,
    slug: raw.slug,
    name: raw.name,
    version: raw.version,
    kind: raw.kind,
    enabled: raw.enabled,
    status: raw.status,
    frontendBundleRelpath: raw.frontend_bundle_relpath,
    minAppVersion: raw.min_app_version,
    maxAppVersion: raw.max_app_version,
    createdAt: raw.created_at,
    updatedAt: raw.updated_at,
    mcpTools: raw.mcp_tools,
    metadata: raw.metadata ?? null,
    requiredTools: (raw.required_tools ?? []).map((r) => ({
      slug: r.slug,
      minVersion: r.min_version ?? '',
      maxVersion: r.max_version ?? '',
    })),
  }
}

export async function fetchTools(): Promise<Tool[]> {
  const raw = await apiGet<{ message: ToolRaw[] }>('/api/v1/tools')
  return raw.message.map(fromRaw)
}

// Field name must be exactly "package" — matches the backend's
// InstallHandler (plan/ai/tools/step-03-install-update-uninstall.md).
export async function installTool(file: File): Promise<Tool> {
  const formData = new FormData()
  formData.append('package', file)
  const raw = await apiUpload<{ message: ToolRaw }>('/api/v1/tools/install', formData)
  return fromRaw(raw.message)
}

export async function enableTool(slug: string): Promise<Tool> {
  const raw = await apiPost<{ message: ToolRaw }>(`/api/v1/tools/${encodeURIComponent(slug)}/enable`)
  return fromRaw(raw.message)
}

export interface ToolDependent {
  slug: string
  name: string
}

// Enabled tools need this one: disabling or deleting is refused until the person confirms (then the call is made again with force).
export class HasDependentsError extends Error {
  dependents: ToolDependent[]
  constructor(dependents: ToolDependent[]) {
    super('Enabled tools need this one.')
    this.dependents = dependents
  }
}

function asDependentsError(err: unknown): unknown {
  if (err instanceof ApiError && err.status === 409) {
    const m = (err.body as { message?: { code?: string; dependents?: ToolDependent[] } } | undefined)?.message
    if (m && typeof m === 'object' && m.code === 'has_dependents' && Array.isArray(m.dependents))
      return new HasDependentsError(m.dependents)
  }
  return err
}

export async function disableTool(slug: string, force = false): Promise<Tool> {
  try {
    const raw = await apiPost<{ message: ToolRaw }>(`/api/v1/tools/${encodeURIComponent(slug)}/disable${force ? '?force=true' : ''}`)
    return fromRaw(raw.message)
  } catch (err) {
    throw asDependentsError(err)
  }
}

export async function deleteTool(slug: string, force = false): Promise<void> {
  try {
    await apiDelete(`/api/v1/tools/${encodeURIComponent(slug)}${force ? '?force=true' : ''}`)
  } catch (err) {
    throw asDependentsError(err)
  }
}
