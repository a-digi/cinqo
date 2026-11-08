// importAiChoice.ts — which AI platform and model one import uses (plan 212).
//
// The Career tool has one saved AI choice (Settings, or the automatic pick). The Import page starts from it and lets the person pick another platform and
// model for this import (and its "Process again"); that pick is not saved and not remembered. This is the pure rule for what is in effect, so it can be
// script-checked without React.
import { defaultModelFor } from '../../Cinqo/Platform/aiChoice'
import type { Platform } from '../../Cinqo/Platform/platformRepository'

export interface ImportAiOverride {
  platformId: string
  model: string
}

export interface ImportAiSettings {
  platform: Platform | null
  model: string | null
}

export type ImportAiSource =
  // The Career AI settings are in effect: the person picked nothing, or picked exactly what the settings say.
  | 'settings'
  // The person's own pick is in effect.
  | 'override'
  // The person picked a platform that can no longer be used (its key was deleted, the local AI was switched off): the settings are used again.
  | 'settings-after-override'

export interface ImportAiChoice extends ImportAiSettings {
  source: ImportAiSource
}

// settings: what useAiChoice resolves. usable: the platforms that can be used (usablePlatforms), in that order. override: the person's pick, or null.
export function resolveImportAi(settings: ImportAiSettings, usable: Platform[], override: ImportAiOverride | null): ImportAiChoice {
  const wanted = (override?.platformId ?? '').trim()
  if (override === null || wanted === '') return { ...settings, source: 'settings' }
  const platform = usable.find((p) => p.id === wanted)
  if (!platform) return { ...settings, source: 'settings-after-override' }
  // The person's model if the platform still offers it, else the platform's first one (null when it lists none).
  const model = platform.models.includes(override.model) ? override.model : defaultModelFor(platform) || null
  const sameAsSettings = platform.id === settings.platform?.id && model === settings.model
  return { platform, model, source: sameAsSettings ? 'settings' : 'override' }
}
