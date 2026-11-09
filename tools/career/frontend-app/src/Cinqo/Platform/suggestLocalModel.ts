// suggestLocalModel.ts (plan 220) — Career tells Cinqo which local model it would like, and Cinqo does the rest: it asks the person on its Local Agents
// page, downloads the model into the place where all models are, and offers it to every picker. Career never downloads anything. The call is made
// with the signed-in person's own session, so it needs the same permission as confirming it (the platform manage scope of Cinqo).
// Pure apart from the one request, so the texts can be script-checked.
import { CoreApiError, httpClient } from '../Http/client'
import { RECOMMENDED_LOCAL_CRAWL_MODEL } from './modelLabels'

// Where the recommended crawl model is: Qwen2.5-Coder-3B, one of the models that ship with Cinqo (repository and file as Cinqo lists them).
export const QWEN_CODER_REPO = 'Qwen/Qwen2.5-Coder-3B-Instruct-GGUF'
export const QWEN_CODER_FILE = 'qwen2.5-coder-3b-instruct-q4_k_m.gguf'

// What became of the suggestion: 'suggested' (it waits for the person on Cinqo's Local Agents page), 'installed' (already downloaded),
// 'downloading' (a download is running), 'dismissed' (the person said no earlier: it stays no).
export type SuggestOutcome = 'suggested' | 'installed' | 'downloading' | 'dismissed'

export interface LocalModelSuggestion {
  id: string
  status: string
  created: boolean
}

export function outcomeOf(s: LocalModelSuggestion): SuggestOutcome {
  switch (s.status) {
    case 'installed':
    case 'downloading':
    case 'dismissed':
      return s.status
    default:
      return 'suggested'
  }
}

// What to tell the person after a suggestion went through. The ones that wait are sent to Cinqo's page, so the others only need a line.
export function outcomeText(o: SuggestOutcome): string {
  switch (o) {
    case 'installed':
      return 'Qwen2.5-Coder-3B is already downloaded in Cinqo.'
    case 'downloading':
      return 'Cinqo is already downloading Qwen2.5-Coder-3B.'
    case 'dismissed':
      return 'You dismissed Qwen2.5-Coder-3B in Cinqo earlier. You can still download it on the Local Agents page.'
    case 'suggested':
      return 'Cinqo is waiting for your confirmation on the Local Agents page.'
  }
}

// Whether the person is sent on to Cinqo's Local Agents page: where a waiting suggestion is confirmed, and where the others can be looked at.
export function opensLocalAgents(o: SuggestOutcome): boolean {
  return o !== 'installed'
}

// Why a suggestion failed, in words for the person.
export function suggestErrorText(err: unknown): string {
  if (err instanceof CoreApiError) {
    if (err.status === 401 || err.status === 403) {
      return 'You need permission to manage the AI platforms in Cinqo to ask it to download a model. Ask an administrator, or download the model yourself on the Local Agents page.'
    }
    if (err.status === 429)
      return 'Too many model suggestions are waiting in Cinqo. Decide about some of them on the Local Agents page first.'
    if (err.status === 502) return 'Cinqo could not reach Hugging Face. Try again in a moment.'
    return err.message
  }
  return err instanceof Error ? err.message : String(err)
}

// Asks Cinqo to download Qwen2.5-Coder-3B. Resolves with what became of the suggestion; rejects with a CoreApiError.
export async function suggestQwenCoder(): Promise<SuggestOutcome> {
  const res = await httpClient.post('/api/v1/local-ai/suggestions', {
    repo: QWEN_CODER_REPO,
    file: QWEN_CODER_FILE,
    suggestedBy: 'Career',
    why: 'The best local model for writing crawl instructions for job links.',
  })
  return outcomeOf(await httpClient.responseBody<LocalModelSuggestion>(res, 'ask Cinqo for the model'))
}

// The id of that model in Cinqo's list, for the checks that it is the one Career recommends.
export const QWEN_CODER_MODEL_ID = RECOMMENDED_LOCAL_CRAWL_MODEL

// Whether the note under a local platform's model picker offers to ask Cinqo for Qwen2.5-Coder-3B: when the platform does not list it (it is not
// downloaded, or does not fit this computer), whatever else the person has (a key, another local model). Only the local platform has such a note.
export function needsQwenCoderDownload(platform: { models: string[] }): boolean {
  return !platform.models.includes(QWEN_CODER_MODEL_ID)
}
