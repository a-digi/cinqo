// suggestedModels.ts (plan 221) — the local models Career suggests to Cinqo, in one list, with what Career wants each of them for. Pure (no React, no
// request), so the texts can be script-checked. The model ids are the ones Cinqo's list uses; Career only ever says *where* a model is (its repository
// and file on Hugging Face): Cinqo knows what it measured about it, asks the person, and does the download.
import { RECOMMENDED_LOCAL_CRAWL_MODEL } from './modelLabels'

export interface SuggestedModel {
  // The id of the model in Cinqo's list (and in the local platform's models once it is downloaded).
  id: string
  // A short name for lines of text ("Qwen2.5-Coder-3B").
  name: string
  repo: string
  file: string
  // What Career suggests it for, in a sentence for the Settings page.
  purpose: string
  // The note under a model picker when it is missing: "<noteLead> is not downloaded."
  noteLead: string
  // The reason Career gives Cinqo with the suggestion (shown to the person on Cinqo's page as Career's own words).
  why: string
}

export const QWEN_CODER: SuggestedModel = {
  id: RECOMMENDED_LOCAL_CRAWL_MODEL,
  name: 'Qwen2.5-Coder-3B',
  repo: 'Qwen/Qwen2.5-Coder-3B-Instruct-GGUF',
  file: 'qwen2.5-coder-3b-instruct-q4_k_m.gguf',
  purpose: 'Writing crawl instructions for job links: it reads a job page and writes the rules the crawler then follows.',
  noteLead: 'Qwen2.5-Coder-3B, the best local model for crawl instructions,',
  why: 'The best local model for writing crawl instructions for job links.',
}

export const PHI_4_MINI: SuggestedModel = {
  id: 'phi-4-mini-instruct-q4_k_m',
  name: 'Phi-4-mini',
  repo: 'unsloth/Phi-4-mini-instruct-GGUF',
  file: 'Phi-4-mini-instruct-Q4_K_M.gguf',
  purpose: 'Importing a CV into your profile with a small model: Career does the steps itself and asks the model only short questions.',
  noteLead: 'Phi-4-mini, the local model for importing a CV,',
  why: 'A small local model that can import a CV: Career does the steps itself and asks it only short questions.',
}

// In the order the Settings page shows them.
export const SUGGESTED_MODELS: SuggestedModel[] = [QWEN_CODER, PHI_4_MINI]

// The suggested models a platform does not list (not downloaded, or not confirmed in Cinqo yet, or they do not fit this computer), in list order.
export function missingModels(platform: { models: string[] }): SuggestedModel[] {
  return SUGGESTED_MODELS.filter((m) => !platform.models.includes(m.id))
}
