import type { InstructionKindStats } from '../../api'
import { attemptsLabel, formatInstructionStats, instructionStatsTooltip } from '../instructionStats'

interface InstructionAttemptStatsProps {
  stats: InstructionKindStats | undefined
  // alertAttempts is instructionAlertAttempts' own result for this
  // kind — 0 means no alert.
  alertAttempts: number
}

// InstructionAttemptStats (step 86) — one instruction document's
// attempt counters ("3 attempts · 1 succeeded · 2 failed", latest
// failure in the tooltip) plus, when attempts were made but there are
// still no instructions, a red alert pill. Stateless; renders nothing
// until there is something to show.
export function InstructionAttemptStats({ stats, alertAttempts }: InstructionAttemptStatsProps) {
  const summary = formatInstructionStats(stats)
  if (!summary && alertAttempts === 0) return null
  return (
    <div className="mt-1 flex flex-col items-center gap-1">
      {summary && (
        <span className="text-[11px] text-gray-500" title={instructionStatsTooltip(stats)}>
          {summary}
        </span>
      )}
      {alertAttempts > 0 && (
        <span
          role="status"
          title={instructionStatsTooltip(stats)}
          className="rounded-full bg-red-100 px-2 py-0.5 text-xs font-medium text-red-800"
        >
          No instructions after {attemptsLabel(alertAttempts)}
        </span>
      )}
    </div>
  )
}
