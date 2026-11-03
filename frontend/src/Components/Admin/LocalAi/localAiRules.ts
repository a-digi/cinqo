import type { LocalAiModel, LocalAiSettings } from './localAiApi'

// What the settings page lets an admin edit, and the rules for it. Pure (no React), so the rules can be script-checked.
// The server checks everything again: these rules only save a round trip and say what is wrong next to the field.
//
// The local AI is managed by Cinqo only: an admin switches it on and chooses one model of the catalog. The context size
// and the idle time are kept as stored (they get controls with the managed runtime).

export interface Draft {
  enabled: boolean
  modelId: string
}

export interface DraftProblems {
  modelId?: string
}

export function draftOf(settings: LocalAiSettings): Draft {
  return { enabled: settings.enabled, modelId: settings.modelId }
}

export function isDirty(settings: LocalAiSettings, draft: Draft): boolean {
  return settings.enabled !== draft.enabled || settings.modelId !== draft.modelId
}

export function problemsOf(draft: Draft, catalog: LocalAiModel[]): DraftProblems {
  if (draft.modelId !== '' && !catalog.some((m) => m.id === draft.modelId)) {
    return { modelId: 'Choose one of the models Cinqo offers.' }
  }
  if (draft.enabled && draft.modelId === '') {
    return { modelId: 'Choose a model to switch the local AI on.' }
  }
  return {}
}

export function hasProblems(problems: DraftProblems): boolean {
  return problems.modelId !== undefined
}

// Whether the switch may be used: it can always be turned off, and turned on only when the local AI can run at all.
export function switchLocked(draft: Draft, canEnable: boolean): boolean {
  return !draft.enabled && !canEnable
}

// The settings to send: the draft's two values and the two numbers kept as stored.
export function toUpdate(settings: LocalAiSettings, draft: Draft): LocalAiSettings {
  return { enabled: draft.enabled, modelId: draft.modelId, contextTokens: settings.contextTokens, idleMinutes: settings.idleMinutes }
}
