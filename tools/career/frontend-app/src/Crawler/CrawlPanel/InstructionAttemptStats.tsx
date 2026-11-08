import type { InstructionKindStats } from '../../api'
import { attemptsLabel, formatInstructionStats, instructionStatsTooltip, uncrawlableTooltip } from '../instructionStats'

interface InstructionAttemptStatsProps {
  stats: InstructionKindStats | undefined
  // alertAttempts is instructionAlertAttempts' own result for this
  // kind — 0 means no alert.
  alertAttempts: number
  // uncrawlable (step 87) — isLikelyUncrawlable's own result; when true
  // the "probably can't be crawled" pill is shown INSTEAD of the generic
  // "no instructions after N attempts" one.
  uncrawlable?: boolean
}

// InstructionAttemptStats (step 86) — one instruction document's
// attempt counters ("3 attempts · 1 succeeded · 2 failed", latest
// failure in the tooltip) plus, when attempts were made but there are
// still no instructions, a red alert pill. Stateless; renders nothing
// until there is something to show.
export function InstructionAttemptStats({ stats, alertAttempts, uncrawlable = false }: InstructionAttemptStatsProps) {
  const summary = formatInstructionStats(stats)
  if (!summary && alertAttempts === 0 && !uncrawlable) return null
  return (
    <div className="mt-1 flex flex-col items-center gap-1">
      {summary && (
        <span className="text-[11px] text-gray-500" title={instructionStatsTooltip(stats)}>
          {summary}
        </span>
      )}
      {uncrawlable && (
        <>
          <span
            role="status"
            title={uncrawlableTooltip(stats)}
            className="rounded-full bg-red-100 px-2 py-0.5 text-xs font-medium text-red-800"
          >
            Probably can&apos;t be crawled
          </span>
          <span className="text-[11px] text-gray-500">Add its jobs manually</span>
        </>
      )}
      {!uncrawlable && alertAttempts > 0 && (
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
