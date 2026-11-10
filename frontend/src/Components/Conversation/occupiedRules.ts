// Pure rules for the answer a start gets when another local model is loaded (plan/ai/conversation/step-241-one-local-model-at-a-time-notify-before-a-new-conversation.md):
// reading the 409 `local_model_occupied`, what the dialog says and offers, and the ask-then-retry flow around a start. No React, so it can be script-checked.
import { ApiError } from '../../api/client'

export interface OccupiedConversation {
  id: string
  title: string
}

export interface OccupiedInfo {
  message: string
  loadedModelName: string
  state: 'idle' | 'loading' | 'in_use'
  conversations: OccupiedConversation[]
  othersUsing: number
  canRelease: boolean
}

export const OCCUPIED_CODE = 'local_model_occupied'

function isRecord(v: unknown): v is Record<string, unknown> {
  return typeof v === 'object' && v !== null
}

// The occupancy answer of a failed start, or null for any other failure (a 409 for another reason, a different status, a body of another shape).
export function parseOccupied(err: unknown): OccupiedInfo | null {
  if (!(err instanceof ApiError) || err.status !== 409 || !isRecord(err.body) || err.body.code !== OCCUPIED_CODE) return null
  const data = err.body.data
  const message = typeof err.body.message === 'string' ? err.body.message : ''
  if (!isRecord(data) || message === '') return null
  const state = data.state
  if (state !== 'idle' && state !== 'loading' && state !== 'in_use') return null
  const conversations: OccupiedConversation[] = Array.isArray(data.conversations)
    ? data.conversations.flatMap((c: unknown) =>
        isRecord(c) && typeof c.id === 'string' && typeof c.title === 'string' ? [{ id: c.id, title: c.title }] : [],
      )
    : []
  return {
    message,
    loadedModelName: typeof data.loadedModelName === 'string' && data.loadedModelName !== '' ? data.loadedModelName : 'The other model',
    state,
    conversations,
    othersUsing: typeof data.othersUsing === 'number' && data.othersUsing > 0 ? data.othersUsing : 0,
    canRelease: data.canRelease === true,
  }
}

export interface OccupiedDialogView {
  title: string
  message: string
  // The unload button's label; null when the person cannot unload (the dialog only has a close button).
  unloadLabel: string | null
  cancelLabel: string
}

// What the dialog says. The unload is offered only when the server says it may be (the person may unload and nobody uses the model).
export function occupiedDialogView(info: OccupiedInfo): OccupiedDialogView {
  if (info.canRelease) {
    return {
      title: 'Another local model is loaded',
      message: info.message,
      unloadLabel: `Unload ${info.loadedModelName} and continue`,
      cancelLabel: 'Cancel',
    }
  }
  if (info.state === 'in_use') {
    return { title: 'A local model is in use', message: info.message, unloadLabel: null, cancelLabel: 'Close' }
  }
  return {
    title: 'Another local model is loaded',
    message: `${info.message} You are not allowed to unload models: ask someone who manages the AI platforms.`,
    unloadLabel: null,
    cancelLabel: 'Close',
  }
}

// The most times one start asks: the model can be taken by someone else between the question and the retry.
export const MAX_ASKS = 3

// Runs a start. A start that fails because another model is loaded asks the person; "unload" runs it again with release set. Resolves with
// the start's result, or undefined when the person said no. Any other failure is thrown unchanged.
export async function startWithOccupiedCheck<T>(
  start: (release: boolean) => Promise<T>,
  ask: (info: OccupiedInfo) => Promise<boolean>,
): Promise<T | undefined> {
  let release = false
  for (let asked = 0; ; asked++) {
    try {
      return await start(release)
    } catch (err) {
      const info = parseOccupied(err)
      if (info === null || asked >= MAX_ASKS) throw err
      if (!(await ask(info))) return undefined
      release = true
    }
  }
}
