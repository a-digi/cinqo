// Pure rules of the shared list of AI platforms (plan/ai/frontend/step-243-ai-availability-context-chats-follow-keys-and-local-models.md, step 1).
// No React, so they can be script-checked.
import type { Platform } from '../../api/platforms'

// Whether two reads of GET /api/v1/platforms say the same thing, compared by value (an identical read must change nothing and re-render nothing).
// The server lists its platforms and their models in a stable order, so the order is part of the value: a changed order is a change.
export function samePlatforms(a: readonly Platform[] | null, b: readonly Platform[] | null): boolean {
  if (a === null || b === null) return a === b
  if (a.length !== b.length) return false
  return JSON.stringify(a) === JSON.stringify(b)
}

export const PLATFORMS_LOAD_ERROR = 'Failed to load platforms.'

// After a change to the local AI the model program and the files that make a model "ready" settle a moment after the call returns, so the platforms are
// read once more a little later (milliseconds), only for that reason. A change to the API keys is visible at once: no second read (null).
export const LOCAL_AI_FOLLOW_UP_MS = 2000

export function followUpDelay(reason: 'keys' | 'local-ai'): number | null {
  return reason === 'local-ai' ? LOCAL_AI_FOLLOW_UP_MS : null
}
