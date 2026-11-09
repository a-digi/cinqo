// suggestLocalModel.ts (plan 220) — Career tells Cinqo which local model it would like, and Cinqo does the rest: it asks the person on its Local Agents
// page, downloads the model into the place where all models are, and offers it to every picker. Career never downloads anything. The call is made
// with the signed-in person's own session, so it needs the same permission as confirming it (the platform manage scope of Cinqo).
// Pure apart from the one request, so the texts can be script-checked.
import { CoreApiError, httpClient } from '../Http/client'
import { QWEN_CODER, type SuggestedModel } from './suggestedModels'

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
export function outcomeText(o: SuggestOutcome, model: Pick<SuggestedModel, 'name'> = QWEN_CODER): string {
  switch (o) {
    case 'installed':
      return `${model.name} is already downloaded in Cinqo.`
    case 'downloading':
      return `Cinqo is already downloading ${model.name}.`
    case 'dismissed':
      return `You dismissed ${model.name} in Cinqo earlier. You can still download it on the Local Agents page.`
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

// Asks Cinqo to download a model: Career says where it is (repository and file) and why it wants it, nothing else. Resolves with what became of the
// suggestion; rejects with a CoreApiError.
export async function suggestModel(model: SuggestedModel): Promise<SuggestOutcome> {
  const res = await httpClient.post('/api/v1/local-ai/suggestions', {
    repo: model.repo,
    file: model.file,
    suggestedBy: 'Career',
    why: model.why,
  })
  return outcomeOf(await httpClient.responseBody<LocalModelSuggestion>(res, 'ask Cinqo for the model'))
}

// Asks Cinqo to download Qwen2.5-Coder-3B (the advice "Get Qwen Coder" on the Platforms page).
export function suggestQwenCoder(): Promise<SuggestOutcome> {
  return suggestModel(QWEN_CODER)
}

// The id of that model in Cinqo's list, for the checks that it is the one Career recommends.
export const QWEN_CODER_MODEL_ID = QWEN_CODER.id
