// Which AI platform and model the Career tool uses, as pure rules (no React, no fetching, so they can be script-checked; useAiChoice.ts feeds them):
//  - usablePlatforms: the platforms that can really be used, the local AI first;
//  - resolveAiChoice: the choice saved on the Settings page if it is still usable, otherwise the first usable platform (so a ready local AI beats any remote platform).
import type { Platform } from './platformRepository'

export interface SavedChoice {
  platformId: string
  model: string
}

export type ChoiceSource =
  // The saved platform exists: it (and its saved model, if still offered) is used.
  | 'saved'
  // Nothing is saved: the first usable platform (the local AI before a platform with a key).
  | 'automatic'
  // Something is saved but that platform can no longer be used (gone, its key deleted, the local AI switched off): treated as not chosen.
  | 'stale'
  // There is no platform at all.
  | 'none'

export interface AiChoice {
  platform: Platform | null
  // The model to use, or null when the platform lists none.
  model: string | null
  source: ChoiceSource
}

function firstModel(p: Platform): string | null {
  return p.models.length > 0 ? p.models[0] : null
}

// The platforms the Career tool can use, in the order it prefers them. The core lists a platform only when it is available; on top of that:
//  - a platform that needs no key (the local AI, which the core lists only while it is on and has a ready model) is usable;
//  - a platform that needs a key is usable only when it has at least one stored key (keyedPlatformIds), unless the keys could not be loaded (keysKnown false):
//    then nothing can be said about them, and hiding every cloud platform because of one failed request would leave the picker empty, so they are kept;
//  - an absent requiresKey (an older core) counts as "needs a key".
// Order: first the platforms that need no key (the local AI is the more important), then the keyed remote ones, each group in the core's order.
export function usablePlatforms(platforms: Platform[], keyedPlatformIds: string[], keysKnown: boolean): Platform[] {
  const needsKey = (p: Platform): boolean => p.requiresKey !== false
  const usable = platforms.filter((p) => !needsKey(p) || !keysKnown || keyedPlatformIds.includes(p.id))
  return [...usable.filter((p) => !needsKey(p)), ...usable.filter(needsKey)]
}

// platforms: the usable ones (usablePlatforms), in that order.
export function resolveAiChoice(platforms: Platform[], saved: SavedChoice): AiChoice {
  if (platforms.length === 0) return { platform: null, model: null, source: 'none' }

  const savedPlatform = saved.platformId === '' ? undefined : platforms.find((p) => p.id === saved.platformId)
  if (savedPlatform) {
    // A saved model that the platform no longer offers falls back to its first one.
    const model = saved.model !== '' && savedPlatform.models.includes(saved.model) ? saved.model : firstModel(savedPlatform)
    return { platform: savedPlatform, model, source: 'saved' }
  }

  const automatic = platforms[0]
  return { platform: automatic, model: firstModel(automatic), source: saved.platformId === '' ? 'automatic' : 'stale' }
}

// The model a platform should be switched to when the user picks that platform.
export function defaultModelFor(platform: Platform): string {
  return firstModel(platform) ?? ''
}
