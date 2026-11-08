import { apiGet } from '../../api/client'
import { CATCH_UP_LINES, FIRST_LINES } from './tailRules'
import type { LiveBatch, LiveFeed, LiveProvider } from './types'

// The live log routes (plan/ai/file-reader/live-follow.md): a log is named by a provider id and a reference, never by a path; the server
// decides what they mean and whether the caller may follow it.

// The providers the caller may follow, so "Live" is offered only where it works. Empty when the server has none (or the module is removed).
export async function fetchLiveProviders(signal?: AbortSignal): Promise<LiveProvider[]> {
  const raw = await apiGet<{ message: LiveProvider[] }>('/api/v1/live-logs/providers', signal)
  return raw.message
}

export async function pollLiveLog(
  provider: string,
  ref: string,
  cursor: string | undefined,
  lines: number,
  signal?: AbortSignal,
): Promise<LiveBatch> {
  const params = new URLSearchParams({ ref, lines: String(lines) })
  if (cursor !== undefined) params.set('cursor', cursor)
  const raw = await apiGet<{ message: LiveBatch }>(`/api/v1/live-logs/${encodeURIComponent(provider)}/poll?${params.toString()}`, signal)
  return raw.message
}

// The feed of one log: the first poll (no cursor) asks for the last lines, the following ones for up to a batch to catch up on.
export function liveFeedFor(provider: string, ref: string): LiveFeed {
  return {
    poll: (cursor, signal) => pollLiveLog(provider, ref, cursor, cursor === undefined ? FIRST_LINES : CATCH_UP_LINES, signal),
  }
}
