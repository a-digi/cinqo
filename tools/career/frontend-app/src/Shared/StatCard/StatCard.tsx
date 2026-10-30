import type { Accent } from '../../Components/JobsPage/jobsTheme'

// One compact statistics card: a solid-colour icon badge, what it is (with its small
// print) and the number. Used in the column beside the list on the Jobs, Portals and
// Crawl Monitor pages.
export function StatCard({
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
      className="relative overflow-hidden rounded-3xl border p-3.5 shadow-sm"
      style={{ borderColor: accent.soft, background: `linear-gradient(160deg, #ffffff 40%, ${accent.soft} 100%)` }}
    >
      <div className="pointer-events-none absolute -right-5 -top-6 h-16 w-16 rounded-full opacity-15" style={{ background: accent.to }} />
      <div className="relative flex items-center gap-3">
        <span
          className="inline-flex h-10 w-10 shrink-0 items-center justify-center rounded-xl text-white shadow-sm"
          style={{ background: accent.to }}
        >
          {icon}
        </span>
        <div className="min-w-0 flex-1">
          <p className="text-sm text-gray-600">{label}</p>
          <div className="flex flex-wrap items-center gap-x-2 gap-y-0.5 text-xs text-gray-500">
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
        <p className="shrink-0 text-2xl font-bold tracking-tight" style={{ color: accent.text }}>
          {value}
        </p>
      </div>
    </div>
  )
}
