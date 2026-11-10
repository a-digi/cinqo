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
  // true for a platform reached with a stored API key (the cloud platforms): only those get "add a key" buttons and
  // a place in the key form. A platform that needs none brings its own settings page.
  requiresKey: boolean
  // true when the platform is ready as far as keys go: a key is stored for it, or it needs none (the local AI).
  hasKey: boolean
  // false for a platform whose conversations get no tools and run in direct mode only (no composer mode).
  offersTools: boolean
  // Readable names of the models offered now, by model id (the local AI); absent for a platform that gives none.
  modelNames?: Record<string, string>
}

// The platforms a new conversation can be started on: those that have a key (or need none). The Platform keys and How-to pages keep
// using the whole list, to add the first key to a platform that has none (plan/ai/conversation/step-229-platforms-with-a-key-only.md).
export function selectablePlatforms(platforms: Platform[]): Platform[] {
  return platforms.filter((p) => p.hasKey)
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
