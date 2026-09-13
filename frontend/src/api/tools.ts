import { apiDelete, apiGet, apiPost, apiUpload } from './client'

// Same snake_case-to-camelCase *Raw mapping convention as api/auth.ts,
// api/security.ts. install_path/pid are never sent by the backend
// (json:"-" on the Go entity) — nothing server-internal to map here.

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

export async function disableTool(slug: string): Promise<Tool> {
  const raw = await apiPost<{ message: ToolRaw }>(`/api/v1/tools/${encodeURIComponent(slug)}/disable`)
  return fromRaw(raw.message)
}

export async function deleteTool(slug: string): Promise<void> {
  await apiDelete(`/api/v1/tools/${encodeURIComponent(slug)}`)
}
