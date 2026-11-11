import type { AiChoice } from '../Cinqo/Platform/aiChoice'
import { usesCodeDriver } from '../Cinqo/Platform/modelLabels'
import { startLocalCvRun } from './local/startLocalCvRun'
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

export type { StartedCvRun }
