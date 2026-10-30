// Which AI platform and model the Career tool uses, as one pure rule: the choice saved
// on the Settings page if it still exists, otherwise the first platform that has an API
// key, otherwise the first platform at all. Pure (no React, no fetching) so it can be
// script-checked; useAiChoice.ts feeds it.
import type { Platform } from './platformRepository'

export interface SavedChoice {
  platformId: string
  model: string
}

export type ChoiceSource =
  // The saved platform exists: it (and its saved model, if still offered) is used.
  | 'saved'
  // Nothing is saved: the first platform with a key, else the first platform.
  | 'automatic'
  // Something is saved but that platform is gone: treated as not chosen.
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

// keyedPlatformIds: the ids of platforms that have at least one API key.
export function resolveAiChoice(platforms: Platform[], keyedPlatformIds: string[], saved: SavedChoice): AiChoice {
  if (platforms.length === 0) return { platform: null, model: null, source: 'none' }

  const savedPlatform = saved.platformId === '' ? undefined : platforms.find((p) => p.id === saved.platformId)
  if (savedPlatform) {
    // A saved model that the platform no longer offers falls back to its first one.
    const model = saved.model !== '' && savedPlatform.models.includes(saved.model) ? saved.model : firstModel(savedPlatform)
    return { platform: savedPlatform, model, source: 'saved' }
  }

  const automatic = platforms.find((p) => keyedPlatformIds.includes(p.id)) ?? platforms[0]
  return { platform: automatic, model: firstModel(automatic), source: saved.platformId === '' ? 'automatic' : 'stale' }
}

// The model a platform should be switched to when the user picks that platform.
export function defaultModelFor(platform: Platform): string {
  return firstModel(platform) ?? ''
}
