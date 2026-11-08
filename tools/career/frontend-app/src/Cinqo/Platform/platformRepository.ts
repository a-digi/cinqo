// platformRepository.ts — cinqo's own CORE API Platform slice
// (/api/v1/platforms), moved out of the old coreApi.ts once the
// Conversation slice (Cinqo/Conversation/conversation.ts) and the
// shared HTTP/JSON client plumbing (CoreApiError, and httpClient's own
// isSuccess/responseBody methods, Cinqo/Http/client.ts) were split out
// of it (steps 44/45) and this became the only thing left. As opposed
// to api.ts's own PROXY_BASE calls to this tool's own backend, this
// file calls cinqo's shared CORE endpoints directly: this tool's
// bundle runs as plain JS in the same page/origin as the rest of the
// app, so the same session cookie already authenticates these calls,
// and each route already enforces its own scope independently — this
// file adds a new CALLER, not a new capability. See
// plan/ai/tools/career/step-24-manual-crawl-trigger.md.
//
// No 401-retry/session-renewal here (unlike the core app's own
// api/client.ts) — deliberately simple, matching this tool's own
// api.ts convention; a session that expires mid-crawl surfaces as a
// plain error in the crawl result line, same as any other failure.
import { httpClient } from '../Http/client'

export interface Platform {
  id: string
  name: string
  models: string[]
  // True for a platform whose models do better with work split into short, one-action messages than with one long prompt
  // (the local AI): the import is then sent as a sequence of small steps. Absent from an older core: false.
  needsShortSteps?: boolean
  // Models of the platform that get the whole task as one full prompt although the platform needs short steps (Phi-4-mini). Absent: none.
  fullPromptModels?: string[]
  // How much a conversation on the platform holds at once, in tokens (the local AI: its context size). Absent for a platform that
  // states none (the cloud platforms), which is never treated as a limit.
  contextTokens?: number
  // Readable names for the models offered now, by model id (the local AI: "Qwen2.5-Coder-3B-Instruct, 2.0 GiB"). Absent when the platform
  // gives none (the cloud platforms): the id is then the name. Absent from an older core.
  modelNames?: Record<string, string>
  // The tasks a model was measured NOT to manage, by model id (the local AI: the 1.5B Qwen cannot do the CV import): a tool can warn before
  // it starts. Absent when the platform flags none (the cloud platforms). Absent from an older core.
  unsuitableFor?: Record<string, string[]>
}

export async function fetchPlatforms(): Promise<Platform[]> {
  const res = await httpClient.get('/api/v1/platforms')
  return httpClient.responseBody<Platform[]>(res, 'load AI platforms')
}

// PlatformKey (step 61) — only the field PortalsPage's own "prefer a
// platform that already has a key" selection needs; the wire response
// also carries label/maskedKey/createdAt, simply ignored here (never
// under-fetched — same JSON either way, just a narrower TS shape).
// GET /api/v1/platforms/keys needs the exact same cinqo:platform:read
// scope fetchPlatforms above already relies on — no new capability,
// just a new caller. See
// plan/ai/tools/career/step-61-portals-prefer-platform-with-key.md.
export interface PlatformKey {
  platform: string
}

export async function fetchPlatformKeys(): Promise<PlatformKey[]> {
  const res = await httpClient.get('/api/v1/platforms/keys')
  return httpClient.responseBody<PlatformKey[]>(res, 'load AI platform keys')
}
