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
}

export async function fetchPlatforms(): Promise<Platform[]> {
  const res = await httpClient.get('/api/v1/platforms')
  return httpClient.responseBody<Platform[]>(res, 'load AI platforms')
}
