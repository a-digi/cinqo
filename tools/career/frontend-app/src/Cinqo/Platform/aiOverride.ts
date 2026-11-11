// aiOverride.ts — which AI platform and model one run uses when the person may pick another one for it (plan 252).
//
// The Career tool has one saved AI choice (Settings, or the automatic pick; see aiChoice.ts). A dialog starts from it and lets the person pick another platform and
// model for that one run; the pick is not saved and not remembered. This is the pure rule for what is in effect, so it can be checked without React. It is the
// same rule as the Import page's (plan 212), kept here for the screens that are not the import.
import { defaultModelFor } from './aiChoice'
import type { Platform } from './platformRepository'

export interface AiOverride {
  platformId: string
  model: string
}

export interface AiSettingsChoice {
  platform: Platform | null
  model: string | null
}

export type AiOverrideSource =
  // The Career AI settings are in effect: the person picked nothing, or picked exactly what the settings say.
  | 'settings'
  // The person's own pick is in effect.
  | 'override'
  // The person picked a platform that can no longer be used (its key was deleted, the local AI was switched off): the settings are used again.
  | 'settings-after-override'

export interface ResolvedAi extends AiSettingsChoice {
  source: AiOverrideSource
}

// settings: what useAiChoice resolves. usable: the platforms that can be used (usablePlatforms), in that order. override: the person's pick, or null.
export function resolveAiOverride(settings: AiSettingsChoice, usable: Platform[], override: AiOverride | null): ResolvedAi {
  const wanted = (override?.platformId ?? '').trim()
  if (override === null || wanted === '') return { ...settings, source: 'settings' }
  const platform = usable.find((p) => p.id === wanted)
  if (!platform) return { ...settings, source: 'settings-after-override' }
  // The person's model if the platform still offers it, else the platform's first one (null when it lists none).
  const model = platform.models.includes(override.model) ? override.model : defaultModelFor(platform) || null
  const sameAsSettings = platform.id === settings.platform?.id && model === settings.model
  return { platform, model, source: sameAsSettings ? 'settings' : 'override' }
}
