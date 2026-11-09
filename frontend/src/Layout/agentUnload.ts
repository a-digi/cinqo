import { ApiError, apiPost } from '../api/client'
import { AppScopes } from '../config/security/scopes'
import type { AgentTone } from './resourceLevel'

// Unloading the local agent from the bar (plan 225): who may, when it is offered, what the confirmation says, and the call.

// Only a person who may change the local AI is offered the button (the route enforces it too).
export function canUnload(scopes: string[]): boolean {
  return scopes.includes(AppScopes.SuperAdmin) || scopes.includes(AppScopes.PlatformManage)
}

// There is something to unload while a model is loading, working or idle; a failed or absent agent has nothing running.
export function isUnloadable(tone: AgentTone): boolean {
  return tone === 'starting' || tone === 'working' || tone === 'idle'
}

export interface UnloadDialogText {
  title: string
  body: string
  action: string
  // a model that a conversation is using is unloaded only with force (the route answers 409 otherwise)
  force: boolean
}

export function unloadDialogText(tone: AgentTone, name: string, inUse: boolean): UnloadDialogText {
  if (tone === 'working' || inUse) {
    return {
      title: `Unload ${name}?`,
      body: `A conversation is using ${name} right now. Unloading stops its answer.`,
      action: 'Unload anyway',
      force: true,
    }
  }
  return {
    title: `Unload ${name}?`,
    body:
      tone === 'starting'
        ? 'It is still loading. The load stops, and it is loaded again the next time a conversation uses it.'
        : 'It is loaded again the next time a conversation uses it.',
    action: 'Unload',
    force: false,
  }
}

// POST /api/v1/local-ai/unload. Resolves when the model program is stopped; rejects with an ApiError (409: a conversation is using it).
export async function unloadLocalAgent(force: boolean): Promise<void> {
  await apiPost('/api/v1/local-ai/unload', { force })
}

export function isInUse(e: unknown): boolean {
  return e instanceof ApiError && e.status === 409
}

// The name of the running local agent for the dialog.
export function agentName(name: string): string {
  return name !== '' ? name : 'the local model'
}
