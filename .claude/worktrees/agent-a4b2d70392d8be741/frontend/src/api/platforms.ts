import { apiDelete, apiGet, apiPost } from './client'

// Platform's own fields are already lowercase/camelCase-compatible on
// the wire (api/src/platform/entity/platform.go's json tags, added in
// step 5) — no snake_case *Raw mapping needed here, unlike api/tools.ts.

// models is empty (never undefined) when this platform has no
// user-facing model choice — the conversation create flow uses this to
// decide whether to show a model picker at all
// (plan/ai/conversation/step-07-fixed-platform-and-model-per-conversation.md).
export interface Platform {
  id: string
  name: string
  models: string[]
}

export interface PlatformKey {
  id: string
  label: string
  platform: string
  maskedKey: string
  createdAt: string
}

export async function fetchPlatforms(): Promise<Platform[]> {
  const raw = await apiGet<{ message: Platform[] }>('/api/v1/platforms')
  return raw.message
}

export async function fetchPlatformKeys(): Promise<PlatformKey[]> {
  const raw = await apiGet<{ message: PlatformKey[] }>('/api/v1/platforms/keys')
  return raw.message
}

export async function createPlatformKey(input: { label: string; platform: string; key: string }): Promise<PlatformKey> {
  const raw = await apiPost<{ message: PlatformKey }>('/api/v1/platforms/keys', input)
  return raw.message
}

export async function deletePlatformKey(id: string): Promise<void> {
  await apiDelete(`/api/v1/platforms/keys/${encodeURIComponent(id)}`)
}
