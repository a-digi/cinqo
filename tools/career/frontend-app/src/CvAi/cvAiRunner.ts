import type { CvAiRun } from '../api'
import { awaitTurnCompletion, type SendMessageResult } from '../Cinqo/Conversation/conversation'
import type { AiChoice } from '../Cinqo/Platform/aiChoice'
import { usesCodeDriver } from '../Cinqo/Platform/modelLabels'
import { startLocalCvRun } from './local/startLocalCvRun'
import { finishRemoteCvRunWithApi } from './remote/finishRemoteRun'
import { startRemoteCvRun, type StartedCvRun } from './remote/startRemoteCvRun'

// The one place that knows which driver makes a CV (plan 253). The drivers live in their own folders and never import each other; this file is the only one that imports
// them. A model the platform marks with the code driver (`drivers[model] === 'code'`, Phi-4-mini today) cannot follow the long tool prompt, so code does the work and the
// model only chooses the design (local/); every other model takes the one-prompt-with-tools path (remote/).
export type CvAiDriver = 'local' | 'remote'

export function cvAiDriverFor(ai: AiChoice): CvAiDriver {
  return ai.platform && usesCodeDriver(ai.platform, ai.model) ? 'local' : 'remote'
}

export function startCvAiRun(persona: { id: string; name: string }, focus: string, ai: AiChoice): Promise<StartedCvRun> {
  return cvAiDriverFor(ai) === 'local' ? startLocalCvRun(persona, focus, ai) : startRemoteCvRun(persona, focus, ai)
}

// The turn to watch for a run that is still generating when the page opens (after a reload): wait for the AI's turn, then, if the AI handed a choice over, make the CV from it
// (a model-driven run; plan 256). A run that already has its CV is left as it is; a run whose AI handed nothing over rejects, so it ends as failed as before. A code-driven (local)
// run cannot be resumed (its steps lived in the page that was closed) and ends the same way.
export function resumeCvAiRun(run: Pick<CvAiRun, 'id' | 'personaId' | 'conversationId'>): () => Promise<SendMessageResult> {
  return async () => {
    const reply = await awaitTurnCompletion(run.conversationId)
    await finishRemoteCvRunWithApi({ id: run.id, personaId: run.personaId })
    return reply
  }
}

export type { StartedCvRun }
