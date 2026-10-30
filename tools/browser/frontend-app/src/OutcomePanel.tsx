import { Switch } from './Shared/Switch'
import { PAGE_ACCENT } from './Shared/theme'
import type { CrawlLogSummary, ReasonTone } from './crawlLogRules'

const BAR: Record<ReasonTone, string> = { ok: '#16a34a', warn: '#d97706', bad: '#e11d48', neutral: '#94a3b8' }

// The Crawl Logs page's overview: a panel on the right with why the logged crawls stopped
// (a horizontal bar each, coloured by whether that is normal), the pages visited in total, and
// how many crawls had a field that matched nothing, with a switch to show only those.
export function OutcomePanel({
  summary,
  onlyUnmatched,
  onToggleUnmatched,
}: {
  summary: CrawlLogSummary
  onlyUnmatched: boolean
  onToggleUnmatched: (on: boolean) => void
}) {
  return (
    <section className="browser-enter browser-delay-2 rounded-3xl border bg-white p-4 shadow-sm" style={{ borderColor: PAGE_ACCENT.soft }}>
      <h2 className="mb-3 text-sm font-semibold text-gray-800">Why crawls stopped</h2>

      {summary.total === 0 && <p className="text-xs text-gray-500">Nothing to count yet.</p>}
      <ul className="space-y-2.5">
        {summary.reasons.map((r) => (
          <li key={r.code}>
            <div className="mb-1 flex items-baseline justify-between gap-2 text-xs">
              <span className="min-w-0 truncate text-gray-700" title={r.code}>
                {r.label}
              </span>
              <span className="shrink-0 font-bold text-gray-900">{r.count}</span>
            </div>
            <div className="h-2 overflow-hidden rounded-full bg-gray-100">
              <div className="h-full rounded-full" style={{ width: `${r.percent}%`, background: BAR[r.tone] }} />
            </div>
          </li>
        ))}
      </ul>

      {summary.total > 0 && (
        <div className="mt-4 space-y-2 border-t pt-3 text-xs text-gray-600" style={{ borderColor: PAGE_ACCENT.soft }}>
          <div className="flex justify-between">
            <span>Pages visited</span>
            <span className="font-bold text-gray-900">{summary.pages}</span>
          </div>
          <div className="flex justify-between">
            <span>Crawls with fields not found</span>
            <span className={`font-bold ${summary.withUnmatched > 0 ? 'text-red-700' : 'text-gray-900'}`}>{summary.withUnmatched}</span>
          </div>
          {summary.withUnmatched > 0 && <Switch size="sm" checked={onlyUnmatched} onChange={onToggleUnmatched} label="Show only those" />}
        </div>
      )}
    </section>
  )
}
