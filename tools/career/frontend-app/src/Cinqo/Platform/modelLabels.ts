// modelLabels.ts (plan 206) — how a platform's models are shown: by a readable name when the platform gives one (the local AI:
// "Qwen2.5-Coder-3B-Instruct, 2.0 GiB"), by the id otherwise (the cloud platforms). Pure (no React), so it can be script-checked.
import type { Platform } from './platformRepository'

// The id of the platform Cinqo runs on this computer.
export const LOCAL_PLATFORM_ID = 'local'

// The Cinqo page where models are downloaded, switched on and chosen as the default.
export const LOCAL_AI_PATH = '/admin/local-ai'

export function modelLabel(platform: Pick<Platform, 'modelNames'>, id: string): string {
  const name = platform.modelNames?.[id]
  return name !== undefined && name.trim() !== '' ? name : id
}

export function modelOptions(platform: Pick<Platform, 'models' | 'modelNames'>): { value: string; label: string }[] {
  return platform.models.map((m) => ({ value: m, label: modelLabel(platform, m) }))
}

// The note under the model picker of the local platform: it says what switching does and where more models come from.
export const LOCAL_MODEL_NOTE =
  'One local model runs at a time; switching takes a few seconds. Only models downloaded in Cinqo are listed.'

export function showsLocalModelNote(platform: Pick<Platform, 'id'> | null | undefined): boolean {
  return platform?.id === LOCAL_PLATFORM_ID
}

// The task of reading a CV into a profile (Career's Import from CV); the same name the core's catalog uses.
export const CV_IMPORT_TASK = 'cv-import'

// Whether the platform says this model was measured not to manage the task. Unknown (a platform that flags nothing, an older core) is
// never "unsuitable".
export function isUnsuitableFor(platform: Pick<Platform, 'unsuitableFor'> | null | undefined, model: string | null | undefined, task: string): boolean {
  if (!platform || !model) return false
  return platform.unsuitableFor?.[model]?.includes(task) === true
}

// Whether the platform needs work split into short one-action steps (the local platform). A model with the code driver does not use them
// (usesCodeDriver), except as the fallback for a CV the code cannot cut.
export function needsShortStepsFor(platform: Pick<Platform, 'needsShortSteps'> | null | undefined): boolean {
  return platform?.needsShortSteps === true
}

// Whether a long task for this model is run in code, with small questions that need no tools (plan 209). Unknown (an older core, a platform
// that names none) is never the code driver.
export function usesCodeDriver(platform: Pick<Platform, 'drivers'> | null | undefined, model: string | null | undefined): boolean {
  if (!platform || !model) return false
  return platform.drivers?.[model] === 'code'
}
