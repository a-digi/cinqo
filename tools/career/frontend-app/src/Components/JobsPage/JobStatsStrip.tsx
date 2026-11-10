import { BriefcaseIcon } from '../../Shared/Icons/icons'
import { StatCard } from '../../Shared/StatCard/StatCard'
import { ACCENTS, accentFor, initialOf } from './jobsTheme'
import { platformBars, statCardRows, type JobStats, type StatCardRow } from './jobStats'
import type { JobStatsState } from './useJobStats'

// The Jobs page's statistics cards (plan 236): the four headline numbers in a row at the top of the page, and where the jobs come from in the
// column to the right of the list. Both are fed by the page's one useJobStats call (GET /jobs/stats). If the numbers cannot be loaded the top says
// so quietly; the list keeps working.
const GREEN = ACCENTS[1]
const AMBER = ACCENTS[2]
const VIOLET = ACCENTS[4]

function Icon({ children }: { children: React.ReactNode }) {
  return (
    <svg
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.8"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
      className="h-5 w-5"
    >
      {children}
    </svg>
  )
}

const CalendarPlusIcon = () => (
  <Icon>
    <rect x="3.5" y="5" width="17" height="15" rx="3" />
    <path d="M8 3v4M16 3v4M3.5 10h17M12 13v4M10 15h4" />
  </Icon>
)
const TargetIcon = () => (
  <Icon>
    <circle cx="12" cy="12" r="8.5" />
    <circle cx="12" cy="12" r="4.5" />
    <circle cx="12" cy="12" r="0.8" fill="currentColor" />
  </Icon>
)
const DocumentIcon = () => (
  <Icon>
    <path d="M7 3h7l5 5v13H7z" />
    <path d="M14 3v5h5M10 13h6M10 17h6" />
  </Icon>
)

function PlatformCard({ stats }: { stats: JobStats }) {
  const bars = platformBars(stats.byPlatform)
  return (
    <div className="rounded-3xl border bg-white p-4 shadow-sm" style={{ borderColor: VIOLET.soft }}>
      <h3 className="text-sm font-semibold text-gray-900">Where jobs come from</h3>
      {bars.length === 0 ? (
        <p className="mt-3 text-sm text-gray-400">No jobs yet.</p>
      ) : (
        <ul className="mt-3 space-y-2.5">
          {bars.map((b) => {
            const accent = accentFor(b.name)
            return (
              <li key={b.name} className="flex items-center gap-2.5">
                <span
                  className="inline-flex h-6 w-6 shrink-0 items-center justify-center rounded-full text-xs font-bold text-white"
                  style={{ background: accent.to }}
                  aria-hidden="true"
                >
                  {initialOf(b.name)}
                </span>
                <div className="min-w-0 flex-1">
                  <div className="flex items-baseline justify-between gap-2 text-xs">
                    <span className="truncate font-medium text-gray-700">{b.name}</span>
                    <span className="shrink-0 text-gray-500">{b.count}</span>
                  </div>
                  <div className="mt-1 h-1.5 overflow-hidden rounded-full" style={{ background: accent.soft }}>
                    <div className="career-grow h-full rounded-full" style={{ width: `${b.widthPct}%`, background: accent.to }} />
                  </div>
                </div>
              </li>
            )
          })}
        </ul>
      )}
    </div>
  )
}

// The accent of each card: its own colour, or the score's tone for the average match.
const ROW_ACCENT: Record<StatCardRow['accent'], (typeof ACCENTS)[number]> = {
  blue: ACCENTS[0],
  violet: VIOLET,
  neutral: ACCENTS[6],
  high: GREEN,
  mid: AMBER,
  low: ACCENTS[3],
  cv: ACCENTS[5],
}
const ROW_ICON: Record<StatCardRow['key'], React.ReactNode> = {
  total: <BriefcaseIcon className="h-5 w-5" />,
  week: <CalendarPlusIcon />,
  match: <TargetIcon />,
  cv: <DocumentIcon />,
}

// The four headline cards in a row: one column on a phone, two from the small breakpoint, four from the extra large one (a window of 1280 px or
// more; narrower, each card is too narrow for its small print).
export function JobStatCards({ jobStats }: { jobStats: JobStatsState }) {
  if (jobStats.state === 'failed') return <p className="mb-5 text-xs text-gray-400">The job statistics could not be loaded.</p>
  if (jobStats.state === 'loading') {
    return (
      <div className="mb-5 grid animate-pulse grid-cols-1 gap-3 sm:grid-cols-2 xl:grid-cols-4" aria-label="Loading statistics">
        {[0, 1, 2, 3].map((i) => (
          <div key={i} className="h-[72px] rounded-3xl bg-gray-100" />
        ))}
      </div>
    )
  }
  return (
    <div className="career-enter mb-5 grid grid-cols-1 gap-3 sm:grid-cols-2 xl:grid-cols-4">
      {statCardRows(jobStats.stats).map((row) => (
        <StatCard
          key={row.key}
          accent={ROW_ACCENT[row.accent]}
          icon={ROW_ICON[row.key]}
          label={row.label}
          value={row.value}
          caption={row.caption}
          trend={row.trend}
        />
      ))}
    </div>
  )
}

// "Where jobs come from", for the column beside the list; nothing while the numbers load or when they cannot be loaded.
export function JobPlatformCard({ jobStats }: { jobStats: JobStatsState }) {
  if (jobStats.state !== 'ready') return null
  return <PlatformCard stats={jobStats.stats} />
}
