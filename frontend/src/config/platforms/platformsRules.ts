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

// How often the platforms are read while a chat shows that no AI is available (a key or a model may be added any moment), and only while the tab is visible.
export const POLL_INTERVAL_MS = 5000

// Coming back to the window fires `focus` and `visibilitychange` together, and a click on the page fires `focus` again: the platforms are read once for such a
// burst, not once per event. now and last are times in milliseconds (last: when the last read for a return was asked for, or null).
export const MIN_RETURN_GAP_MS = 1000

export function shouldReadOnReturn(now: number, last: number | null): boolean {
  return last === null || now - last >= MIN_RETURN_GAP_MS
}

// What a new conversation's form works with: the platform and model picked (the model is empty for a platform with no model choice).
export interface PlatformChoice {
  platformId: string
  model: string
}

// The choice the form should show and submit, given the platforms a conversation can be started on now (the selectable ones) and what the person picked.
// A pick that still exists is kept (a model that is no longer offered gives way to the platform's first one); a platform that is gone gives way to the
// first platform and its first model; with none left the choice is empty. The person's own pick is not forgotten by the caller: if the platform comes
// back, the pick is valid again.
export function reconcileChoice(selectable: readonly Platform[], choice: PlatformChoice): PlatformChoice {
  const picked = selectable.find((p) => p.id === choice.platformId)
  if (picked !== undefined) {
    if (picked.models.length === 0) return { platformId: picked.id, model: '' }
    return { platformId: picked.id, model: picked.models.includes(choice.model) ? choice.model : (picked.models[0] ?? '') }
  }
  const first = selectable.at(0)
  if (first === undefined) return { platformId: '', model: '' }
  return { platformId: first.id, model: first.models[0] ?? '' }
}

export type ConversationAvailability = { available: true } | { available: false; reason: string }

// Whether the platform and model a conversation was created with can still answer. platforms is null until the first read: nothing is known then and the
// conversation is not blocked. A platform that needs a key and has none, a platform that is gone, and a model the platform no longer offers are the
// reasons; a platform that offers no model choice (models empty) is judged by its key only.
export function conversationAvailability(
  platforms: readonly Platform[] | null,
  conversation: { platformId: string; model: string },
): ConversationAvailability {
  if (platforms === null) return { available: true }
  const platform = platforms.find((p) => p.id === conversation.platformId)
  if (platform === undefined) {
    return {
      available: false,
      reason:
        conversation.platformId === 'local'
          ? 'The local AI is not available: it is switched off or has no model ready.'
          : `The AI platform "${conversation.platformId}" is not available any more.`,
    }
  }
  if (platform.requiresKey && !platform.hasKey) return { available: false, reason: `The API key of ${platform.name} was removed.` }
  if (platform.models.length > 0 && !platform.models.includes(conversation.model)) {
    const name = platform.modelNames?.[conversation.model] ?? conversation.model
    return {
      available: false,
      reason: platform.requiresKey
        ? `The model ${name} is not offered by ${platform.name} any more.`
        : `The model ${name} is not available any more: it was removed, or the local AI is switched off.`,
    }
  }
  return { available: true }
}
