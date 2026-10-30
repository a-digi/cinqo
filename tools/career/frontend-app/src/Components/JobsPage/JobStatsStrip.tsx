import { useEffect, useState } from 'react'
import { fetchJobStatsRaw } from '../../api'
import { BriefcaseIcon } from '../../Shared/Icons/icons'
import { ACCENTS, accentFor, initialOf, type Accent } from './jobsTheme'
import { dayChart, parseJobStats, percentOf, platformBars, scoreTone, weekTrend, type ChartBox, type JobStats } from './jobStats'

// The Jobs page's dashboard strip: four headline numbers, a 14-day chart of how many
// jobs were added, and where the jobs come from. Fed by GET /jobs/stats; refetched
// when `refreshKey` changes (a job was added, deleted, matched…). If the numbers
// cannot be loaded the strip says so quietly; the page below keeps working.
const BLUE = ACCENTS[0]
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

function StatCard({
  accent,
  icon,
  label,
  value,
  caption,
  trend,
}: {
  accent: Accent
  icon: React.ReactNode
  label: string
  value: string
  caption: string
  trend?: { arrow: string; text: string; tone: 'good' | 'neutral' } | null
}) {
  return (
    <div
      className="relative overflow-hidden rounded-3xl border p-4 shadow-sm"
      style={{ borderColor: accent.soft, background: `linear-gradient(160deg, #ffffff 40%, ${accent.soft} 100%)` }}
    >
      <div className="pointer-events-none absolute -right-5 -top-5 h-20 w-20 rounded-full opacity-15" style={{ background: accent.to }} />
      <div className="relative flex items-center gap-2.5">
        <span
          className="inline-flex h-9 w-9 shrink-0 items-center justify-center rounded-xl text-white shadow-sm"
          style={{ background: accent.to }}
        >
          {icon}
        </span>
        <span className="text-sm text-gray-500">{label}</span>
      </div>
      <p className="relative mt-2 text-3xl font-bold tracking-tight" style={{ color: accent.text }}>
        {value}
      </p>
      <div className="relative mt-1 flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-gray-500">
        {trend && (
          <span
            className="inline-flex items-center gap-1 rounded-full px-2 py-0.5 font-medium"
            style={trend.tone === 'good' ? { background: '#dcfce7', color: '#15803d' } : { background: '#f3f4f6', color: '#6b7280' }}
          >
            {trend.arrow} {trend.text}
          </span>
        )}
        {caption && <span>{caption}</span>}
      </div>
    </div>
  )
}

const chartBox: ChartBox = { width: 460, height: 150, padX: 6, padTop: 18, padBottom: 20 }

function DayChartCard({ stats }: { stats: JobStats }) {
  const chart = dayChart(stats.perDay, chartBox)
  return (
    <div className="rounded-3xl border bg-white p-4 shadow-sm lg:col-span-2" style={{ borderColor: BLUE.soft }}>
      <div className="flex items-baseline justify-between gap-2">
        <h3 className="text-sm font-semibold text-gray-900">Jobs added</h3>
        <span className="text-xs text-gray-400">last {stats.perDay.length} days</span>
      </div>
      <svg viewBox={`0 0 ${chartBox.width} ${chartBox.height}`} role="img" aria-label="Jobs added per day" className="mt-2 h-auto w-full">
        <line
          x1={chartBox.padX}
          x2={chartBox.width - chartBox.padX}
          y1={chart.baselineY}
          y2={chart.baselineY}
          stroke="#e5e7eb"
          strokeDasharray="3 4"
        />
        {chart.bars.map((b, i) => (
          <g key={i}>
            <rect
              x={b.x}
              y={b.y}
              width={b.w}
              height={Math.max(b.h, 0)}
              rx={Math.min(6, b.w / 2)}
              fill={BLUE.to}
              opacity={b.recent ? 1 : 0.4}
            />
            {b.count > 0 && (
              <text x={b.x + b.w / 2} y={b.y - 4} textAnchor="middle" fontSize={10} fontWeight={600} fill="#374151">
                {b.count}
              </text>
            )}
          </g>
        ))}
        {chart.labels.map((l) => (
          <text key={`${l.x}-${l.text}`} x={l.x} y={chartBox.height - 5} textAnchor="middle" fontSize={9} fill="#6b7280">
            {l.text}
          </text>
        ))}
      </svg>
      <p className="mt-1 flex items-center gap-3 text-xs text-gray-400">
        <span className="inline-flex items-center gap-1.5">
          <span className="inline-block h-2 w-2 rounded-full" style={{ background: BLUE.to }} /> last 7 days
        </span>
        <span className="inline-flex items-center gap-1.5">
          <span className="inline-block h-2 w-2 rounded-full opacity-40" style={{ background: BLUE.to }} /> the week before
        </span>
      </p>
    </div>
  )
}

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
                    <div className="h-full rounded-full" style={{ width: `${b.widthPct}%`, background: accent.to }} />
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

  if (state === 'failed') return <p className="mb-5 text-xs text-gray-400">The job statistics could not be loaded.</p>
  if (state === 'loading' || !stats) {
    return (
      <div className="mb-5 grid animate-pulse grid-cols-2 gap-3 xl:grid-cols-4" aria-label="Loading statistics">
        {[0, 1, 2, 3].map((i) => (
          <div key={i} className="h-28 rounded-3xl bg-gray-100" />
        ))}
      </div>
    )
  }

  const trend = weekTrend(stats.addedLast7Days, stats.addedPrev7Days)
  const tone = stats.avgMatchScore === null ? null : scoreTone(stats.avgMatchScore)
  // No matches yet is not bad news: a neutral colour, not the red of a low score.
  const scoreAccent = tone === null ? ACCENTS[6] : tone === 'high' ? GREEN : tone === 'mid' ? AMBER : ACCENTS[3]

  return (
    <div className="mb-5 space-y-3">
      <div className="grid grid-cols-2 gap-3 xl:grid-cols-4">
        <StatCard
          accent={BLUE}
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
      </div>
      <div className="grid gap-3 lg:grid-cols-3">
        <DayChartCard stats={stats} />
        <PlatformCard stats={stats} />
      </div>
    </div>
  )
}
