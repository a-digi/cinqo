// cinqoModelState.ts (plan 221) — what Cinqo says about the models Career suggests, so the Settings page can tell the person where each one stands. Read
// with the same read scope as the platform list. Pure rules apart from the one request, so they can be script-checked.
import { httpClient } from '../Http/client'
import type { SuggestedModel } from './suggestedModels'

// downloaded: models Cinqo lists that are on this computer; waiting: suggestions nobody decided about yet; dismissed: the person said no.
export interface CinqoModelState {
  downloaded: string[]
  waiting: string[]
  dismissed: string[]
}

// Reads Cinqo's answer (GET /api/v1/local-ai) as far as it has the fields: an older Cinqo that lacks the suggestions or the dismissed list just has
// none, never an error.
export function parseCinqoState(raw: unknown): CinqoModelState {
  const o = (raw ?? {}) as {
    catalog?: { id?: string; install?: { state?: string } }[]
    suggestions?: { id?: string }[]
    dismissed?: string[]
  }
  const ids = (list: { id?: string }[] | undefined) => (list ?? []).map((x) => x.id).filter((id): id is string => typeof id === 'string')
  return {
    downloaded: ids((o.catalog ?? []).filter((m) => m.install?.state === 'ready')),
    waiting: ids(o.suggestions),
    dismissed: (o.dismissed ?? []).filter((id): id is string => typeof id === 'string'),
  }
}

// null when Cinqo cannot be asked (no permission to read, Cinqo not reachable): the card then still shows what the platform lists and still offers the button.
export async function fetchCinqoModelState(): Promise<CinqoModelState | null> {
  try {
    const res = await httpClient.get('/api/v1/local-ai')
    if (!res.ok) return null
    const body = (await res.json()) as { message?: unknown }
    return parseCinqoState(body.message)
  } catch {
    return null
  }
}

// Where one suggested model stands: downloaded (the platform lists it, or Cinqo has it on this computer), waiting for the person's confirmation in Cinqo,
// dismissed by the person, or missing.
export type ModelRowState = 'downloaded' | 'waiting' | 'dismissed' | 'missing'

export function rowState(model: Pick<SuggestedModel, 'id'>, platformModels: string[], cinqo: CinqoModelState | null): ModelRowState {
  if (platformModels.includes(model.id) || cinqo?.downloaded.includes(model.id)) return 'downloaded'
  if (cinqo?.waiting.includes(model.id)) return 'waiting'
  if (cinqo?.dismissed.includes(model.id)) return 'dismissed'
  return 'missing'
}

export function rowStateText(state: ModelRowState): string {
  switch (state) {
    case 'downloaded':
      return 'Downloaded in Cinqo'
    case 'waiting':
      return 'Waiting for your confirmation in Cinqo'
    case 'dismissed':
      return 'You dismissed it in Cinqo. You can still download it on the Local Agents page.'
    case 'missing':
      return 'Not downloaded'
  }
}

// Career asks only for a model that is missing: one that waits, was dismissed, or is there needs no request (a dismissed one stays dismissed).
export function canAsk(state: ModelRowState): boolean {
  return state === 'missing'
}

// Whether the row points to Cinqo's Local Agents page instead.
export function pointsToCinqo(state: ModelRowState): boolean {
  return state === 'waiting' || state === 'dismissed'
}
