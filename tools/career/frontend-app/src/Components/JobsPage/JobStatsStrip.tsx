import { useEffect, useState } from 'react'
import { fetchJobStatsRaw } from '../../api'
import { BriefcaseIcon } from '../../Shared/Icons/icons'
import { StatCard } from '../../Shared/StatCard/StatCard'
import { ACCENTS, accentFor, initialOf } from './jobsTheme'
import { parseJobStats, percentOf, platformBars, scoreTone, weekTrend, type JobStats } from './jobStats'

// The Jobs page's statistics cards, stacked in the column to the right of the list:
// four headline numbers and where the jobs come from. Fed by GET /jobs/stats;
// refetched when `refreshKey` changes (a job was added, deleted, matched…). If the
// numbers cannot be loaded the column says so quietly; the list keeps working.
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

export function JobStatsStrip({ refreshKey }: { refreshKey: string }) {
  const [stats, setStats] = useState<JobStats | null>(null)
  const [state, setState] = useState<'loading' | 'ready' | 'failed'>('loading')

  useEffect(() => {
    let cancelled = false
    fetchJobStatsRaw()
      .then((raw) => {
        if (cancelled) return
        const parsed = parseJobStats(raw)
        setStats(parsed)
        setState(parsed ? 'ready' : 'failed')
      })
      .catch(() => {
        if (!cancelled) setState((prev) => (prev === 'ready' ? prev : 'failed'))
      })
    return () => {
      cancelled = true
    }
  }, [refreshKey])

  if (state === 'failed') return <p className="text-xs text-gray-400">The job statistics could not be loaded.</p>
  if (state === 'loading' || !stats) {
    return (
      <div className="animate-pulse space-y-3" aria-label="Loading statistics">
        {[0, 1, 2, 3].map((i) => (
          <div key={i} className="h-[72px] rounded-3xl bg-gray-100" />
        ))}
      </div>
    )
  }

  const trend = weekTrend(stats.addedLast7Days, stats.addedPrev7Days)
  const tone = stats.avgMatchScore === null ? null : scoreTone(stats.avgMatchScore)
  // No matches yet is not bad news: a neutral colour, not the red of a low score.
  const scoreAccent = tone === null ? ACCENTS[6] : tone === 'high' ? GREEN : tone === 'mid' ? AMBER : ACCENTS[3]

  return (
    <div className="space-y-3">
      <StatCard
        accent={ACCENTS[0]}
        icon={<BriefcaseIcon className="h-5 w-5" />}
        label="Saved jobs"
        value={stats.total.toLocaleString('en')}
        caption={`${stats.companies} ${stats.companies === 1 ? 'company' : 'companies'}${stats.manual > 0 ? ` · ${stats.manual} added by hand` : ''}`}
      />
      <StatCard
        accent={VIOLET}
        icon={<CalendarPlusIcon />}
        label="Added this week"
        value={stats.addedLast7Days.toLocaleString('en')}
        caption=""
        trend={trend}
      />
      <StatCard
        accent={scoreAccent}
        icon={<TargetIcon />}
        label="Average match"
        value={stats.avgMatchScore === null ? '–' : `${stats.avgMatchScore}%`}
        caption={`${stats.matched} of ${stats.total} matched (${percentOf(stats.matched, stats.total)}%)`}
      />
      <StatCard
        accent={ACCENTS[5]}
        icon={<DocumentIcon />}
        label="With a CV"
        value={stats.withCv.toLocaleString('en')}
        caption={`${percentOf(stats.withCv, stats.total)}% of jobs`}
      />
      <PlatformCard stats={stats} />
    </div>
  )
}
