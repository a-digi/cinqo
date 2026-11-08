import { CONTEXT_BASELINE, contextFits } from './localAiFormat'
import type { LocalAiModel, LocalAiSettings } from './localAiApi'

// What the settings page lets an admin edit, and the rules for it. Pure (no React), so the rules can be script-checked.
// The server checks everything again: these rules only save a round trip and say what is wrong next to the field.
//
// The local AI is managed by Cinqo only: an admin chooses one model of the catalog, how much context it reads, how long it
// stays loaded when unused, and switches it on once that model can be run.

export interface Draft {
  enabled: boolean
  modelId: string
  contextTokens: number
  idleMinutes: number
}

export interface DraftProblems {
  modelId?: string
  contextTokens?: string
  idleMinutes?: string
}

// The same bounds as the server.
export const MIN_CONTEXT = 512
export const MAX_CONTEXT = 131072
export const MAX_IDLE_MINUTES = 1440

const CONTEXT_STEPS = [2048, 4096, 8192, 16384, 32768, 65536, 131072]

export function draftOf(settings: LocalAiSettings): Draft {
  return { enabled: settings.enabled, modelId: settings.modelId, contextTokens: settings.contextTokens, idleMinutes: settings.idleMinutes }
}

export function isDirty(settings: LocalAiSettings, draft: Draft): boolean {
  return (
    settings.enabled !== draft.enabled ||
    settings.modelId !== draft.modelId ||
    settings.contextTokens !== draft.contextTokens ||
    settings.idleMinutes !== draft.idleMinutes
  )
}

function modelOf(draft: Draft, catalog: LocalAiModel[]): LocalAiModel | undefined {
  return catalog.find((m) => m.id === draft.modelId)
}

// The context sizes to offer: the usual steps the chosen model can read, and the current value if it is not one of them (so
// a stored value is never silently replaced).
export function contextOptions(draft: Draft, catalog: LocalAiModel[]): number[] {
  const max = modelOf(draft, catalog)?.contextTokens ?? MAX_CONTEXT
  const steps = CONTEXT_STEPS.filter((n) => n <= max)
  if (Number.isInteger(draft.contextTokens) && !steps.includes(draft.contextTokens)) steps.push(draft.contextTokens)
  return steps.sort((a, b) => a - b)
}

export function problemsOf(draft: Draft, catalog: LocalAiModel[]): DraftProblems {
  const problems: DraftProblems = {}
  const model = modelOf(draft, catalog)
  if (draft.modelId !== '' && !model) {
    problems.modelId = 'Choose one of the models Cinqo offers.'
  } else if (draft.enabled && draft.modelId === '') {
    problems.modelId = 'Choose a model to switch the local AI on.'
  }

  const max = model?.contextTokens ?? MAX_CONTEXT
  if (!Number.isInteger(draft.contextTokens) || draft.contextTokens < MIN_CONTEXT || draft.contextTokens > MAX_CONTEXT) {
    problems.contextTokens = `Choose a context size between ${String(MIN_CONTEXT)} and ${MAX_CONTEXT.toLocaleString('en-US')} tokens.`
  } else if (draft.contextTokens > max) {
    problems.contextTokens = `This model reads at most ${max.toLocaleString('en-US')} tokens: choose a smaller context size.`
  } else if (!contextFits(model, draft.contextTokens)) {
    problems.contextTokens = `This computer does not have the memory for ${draft.contextTokens.toLocaleString('en-US')} tokens with this model: choose at most ${String(model?.largestContext)} tokens.`
  }

  if (!Number.isInteger(draft.idleMinutes) || draft.idleMinutes < 0 || draft.idleMinutes > MAX_IDLE_MINUTES) {
    problems.idleMinutes = `Enter a whole number of minutes from 0 to ${String(MAX_IDLE_MINUTES)}.`
  }
  return problems
}

export function hasProblems(problems: DraftProblems): boolean {
  return problems.modelId !== undefined || problems.contextTokens !== undefined || problems.idleMinutes !== undefined
}

export interface SwitchState {
  locked: boolean
  // Why it cannot be switched on (shown next to the switch); empty when it can, or when it is already on.
  reason: string
}

// Whether the switch may be used. It can always be turned off. It can be turned on when the chosen model can be run now: the
// server answers that for every model (`enable`), so the page never repeats those rules.
export function switchState(draft: Draft, catalog: LocalAiModel[]): SwitchState {
  if (draft.enabled) return { locked: false, reason: '' }
  const model = modelOf(draft, catalog)
  if (!model) return { locked: true, reason: 'Choose a model first.' }
  if (!model.enable.ok)
    return { locked: true, reason: model.enable.reason === '' ? 'This model cannot be run yet.' : `${upperFirst(model.enable.reason)}.` }
  return { locked: false, reason: '' }
}

function upperFirst(s: string): string {
  return s === '' ? s : s.charAt(0).toUpperCase() + s.slice(1)
}

// The settings to send.
export function toUpdate(draft: Draft): LocalAiSettings {
  return { enabled: draft.enabled, modelId: draft.modelId, contextTokens: draft.contextTokens, idleMinutes: draft.idleMinutes }
}

// The draft after the admin picks a model. On a fresh install (no model was ever saved) that still has the default context size, a
// model that can use tools preselects its recommended size, when this computer has the memory for it: the admin sees it and
// saves it (nothing is stored by picking). Every other case only changes the model, so a size the admin chose is never replaced.
export function draftAfterPicking(draft: Draft, picked: LocalAiModel, savedModelId: string): Draft {
  const next = { ...draft, modelId: picked.id }
  const recommended = picked.recommendedContext
  if (savedModelId === '' && draft.contextTokens === CONTEXT_BASELINE && recommended !== null && contextFits(picked, recommended)) {
    next.contextTokens = recommended
  }
  return next
}
