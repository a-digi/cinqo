import type { ReactNode } from 'react'
import type { AllAiTraceLogsEntry } from '../../../api/conversations'
import { CountUp } from '../../../Shared/Components/Motion/CountUp'
import { latestModified, relativeTime, totalFiles } from './aiLogsRules'

function Tile({ label, icon, children }: { label: string; icon: ReactNode; children: ReactNode }) {
  return (
    <div className="flex items-center gap-3 rounded-xl bg-white/70 px-4 py-3 ring-1 ring-gray-200">
      <span className="inline-flex h-10 w-10 shrink-0 items-center justify-center rounded-xl bg-indigo-50 text-indigo-600">{icon}</span>
      <div className="min-w-0">
        <p className="text-xs font-medium uppercase tracking-wide text-gray-500">{label}</p>
        <p className="truncate text-xl font-semibold text-gray-900">{children}</p>
      </div>
    </div>
  )
}

const svg = {
  viewBox: '0 0 24 24',
  className: 'h-5 w-5',
  fill: 'none',
  stroke: 'currentColor',
  strokeWidth: 1.6,
  strokeLinecap: 'round',
  strokeLinejoin: 'round',
} as const

// The three figures above the table: how many conversations have logs, how many files that is, and when
// something last wrote one. Across every entry, not just the filtered ones.
export function AiLogsStats({ entries, loading, now }: { entries: AllAiTraceLogsEntry[]; loading: boolean; now: Date }) {
  const dash = <span className="text-gray-300">—</span>
  return (
    <div className="grid gap-3 sm:grid-cols-3">
      <Tile
        label="Conversations"
        icon={
          <svg {...svg} aria-hidden="true">
            <path d="M5 5h14a1 1 0 0 1 1 1v9a1 1 0 0 1-1 1h-7l-4 3v-3H5a1 1 0 0 1-1-1V6a1 1 0 0 1 1-1z" />
          </svg>
        }
      >
        {loading ? dash : <CountUp value={String(entries.length)} />}
      </Tile>
      <Tile
        label="Log files"
        icon={
          <svg {...svg} aria-hidden="true">
            <path d="M7 3h7l4 4v13a1 1 0 0 1-1 1H7a1 1 0 0 1-1-1V4a1 1 0 0 1 1-1z" />
            <path d="M14 3v4h4M9 12h6M9 16h6" />
          </svg>
        }
      >
        {loading ? dash : <CountUp value={String(totalFiles(entries))} delayMs={150} />}
      </Tile>
      <Tile
        label="Last activity"
        icon={
          <svg {...svg} aria-hidden="true">
            <circle cx="12" cy="12" r="8" />
            <path d="M12 7.5V12l3 2" />
          </svg>
        }
      >
        {loading ? dash : relativeTime(latestModified(entries), now)}
      </Tile>
    </div>
  )
}
