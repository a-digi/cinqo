import { clearanceSegments, formatIn, type DomainSummary } from './domainRules'
import { PAGE_ACCENT } from './Shared/theme'

const COLOR = { valid: '#16a34a', expired: '#d97706', none: '#cbd5e1' } as const

// The Domains page's overview: a band above the list with one segmented bar — how many
// sites are cleared, expired or not cleared, in proportion — a pulsing pill while a
// Chrome window is open for someone, and when the soonest clearance runs out.
export function ClearanceBar({ summary, now }: { summary: DomainSummary; now: Date }) {
  if (summary.total === 0) return null
  const segments = clearanceSegments(summary)
  return (
    <section className="mb-5 rounded-3xl border bg-white p-4 shadow-sm" style={{ borderColor: PAGE_ACCENT.soft }}>
      <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
        <h2 className="text-sm font-semibold text-gray-800">Clearances</h2>
        <div className="flex flex-wrap items-center gap-2 text-xs">
          {summary.waiting > 0 && (
            <span role="status" className="animate-pulse rounded-full bg-blue-100 px-2.5 py-0.5 font-medium text-blue-800">
              {summary.waiting} {summary.waiting === 1 ? 'window is' : 'windows are'} open for you
            </span>
          )}
          {summary.nextExpiry && (
            <span className="text-gray-500">Next clearance expires in {formatIn(summary.nextExpiry.getTime() - now.getTime())}</span>
          )}
        </div>
      </div>

      <div className="flex h-3 w-full overflow-hidden rounded-full bg-gray-100" role="img" aria-label="Sites by clearance state">
        {segments
          .filter((s) => s.count > 0)
          .map((s) => (
            <div key={s.key} style={{ width: `${s.percent}%`, background: COLOR[s.key] }} title={`${s.label}: ${s.count}`} />
          ))}
      </div>

      <ul className="mt-3 flex flex-wrap gap-x-5 gap-y-1 text-xs text-gray-600">
        {segments.map((s) => (
          <li key={s.key} className="flex items-center gap-1.5">
            <span aria-hidden="true" className="h-2.5 w-2.5 rounded-full" style={{ background: COLOR[s.key] }} />
            {s.label} <span className="font-bold text-gray-900">{s.count}</span>
          </li>
        ))}
      </ul>
    </section>
  )
}
