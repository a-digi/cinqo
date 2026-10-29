// modelHint.ts (step 93) — a one-line notice for the Generate controls while
// the "openrouter/free" model is selected. That model is a router: it picks a
// different free model for every call, and the weak ones often fail at tool
// use (malformed arguments, giving up), which is what "Generate instructions"
// is made of. Pure rules, no React. See
// plan/ai/tools/career/step-93-tool-call-budget-and-weak-models.md.
import type { Platform } from '../Cinqo/Platform/platformRepository'

export const FREE_ROUTER_PLATFORM_ID = 'openrouter'
export const FREE_ROUTER_MODEL = 'openrouter/free'

export const FREE_ROUTER_HINT =
  'This model picks a different free model for every call, which often fails at tool use. For crawl instructions, pick a specific model above.'

// effectiveModel is the model a Generate click will actually use: the picked
// one, else the platform's first (the same rule the click handlers apply).
export function effectiveModel(platform: Platform | null, selectedModel: string | null): string | undefined {
  if (!platform || platform.models.length === 0) return undefined
  return selectedModel ?? platform.models[0]
}

export function isFreeRouter(platform: Platform | null, selectedModel: string | null): boolean {
  return platform?.id === FREE_ROUTER_PLATFORM_ID && effectiveModel(platform, selectedModel) === FREE_ROUTER_MODEL
}
