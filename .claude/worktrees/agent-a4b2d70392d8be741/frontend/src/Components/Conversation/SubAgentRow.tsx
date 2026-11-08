import { useState } from 'react'
import type { SubAgentRun } from '../../api/conversations'
import { formatTokenCount } from './formatTokenCount'

// Shared by ThinkingIndicator (the live, currently-running turn's own
// panel) and MessageThread's own historical click-through
// (fetchSubAgentsForTurn) — one row per sub-agent: a status dot
// (running/completed/failed/cancelled, same three-state-plus-running
// convention TurnStatusBadge already established for the parent turn
// itself), its task text, and whatever's relevant to that status (a
// live token count while running, the last log line on failure, or
// the final result — collapsed behind a toggle, same "Show the AI's
// raw reply" convention ImportCvPage.tsx already established — once
// completed). This is the "visually see sub agents working and on
// what" ask, plus the click-through onto what they actually produced.
// See plan/ai/conversation/step-41-sub-agents.md.
const SUB_AGENT_DOT_CLASSES: Record<SubAgentRun['status'], string> = {
  running: 'animate-pulse bg-amber-500',
  completed: 'bg-green-500',
  failed: 'bg-red-500',
  cancelled: 'bg-gray-400',
}

// lastLogStep strips log.go's own "<RFC3339 timestamp>\t<step>" prefix
// — the raw shape is a diagnostic trace, not display text; only the
// step text itself belongs in the UI.
function lastLogStep(log: string[]): string | undefined {
  const line = log[log.length - 1]
  if (!line) return undefined
  const tab = line.indexOf('\t')
  return tab === -1 ? line : line.slice(tab + 1)
}

export function SubAgentRow({ agent }: { agent: SubAgentRun }) {
  const [showResult, setShowResult] = useState(false)

  return (
    <div className="flex items-start gap-1.5 text-xs text-gray-400">
      <span className={`mt-1 h-1.5 w-1.5 shrink-0 rounded-full ${SUB_AGENT_DOT_CLASSES[agent.status]}`} />
      <span className="min-w-0 flex-1">
        <span className="block truncate text-gray-500">{agent.task}</span>
        {agent.status === 'running' && agent.totalTokens > 0 && (
          <span className="tabular-nums">
            ↑{formatTokenCount(agent.promptTokens)} ↓{formatTokenCount(agent.completionTokens)}
          </span>
        )}
        {agent.status === 'failed' && lastLogStep(agent.log) && <span className="block text-red-400">{lastLogStep(agent.log)}</span>}
        {agent.status === 'completed' && agent.result && (
          <>
            <button
              type="button"
              onClick={() => {
                setShowResult((v) => !v)
              }}
              className="underline hover:text-gray-600"
            >
              {showResult ? 'Hide result' : 'Show result'}
            </button>
            {showResult && <pre className="mt-1 whitespace-pre-wrap break-words text-gray-600">{agent.result}</pre>}
          </>
        )}
      </span>
    </div>
  )
}
