import { accentFor, initialOf } from '../JobsPage/jobsTheme'
import { plural } from '../CompaniesPage/companyStats'
import type { PlatformCount } from './profileRules'

// The Profiles page's overview: a shelf of cards, one per platform the profiles link
// to (with how many links), and a dashed card for the profiles that have none. It sits
// above the list, so the list keeps the whole width.
export function PlatformShelf({
  platforms,
  withoutLinks,
  average,
  onShowWithoutLinks,
}: {
  platforms: PlatformCount[]
  withoutLinks: number
  // The average profile completeness in percent, or null when there are no profiles.
  average: number | null
  onShowWithoutLinks: () => void
}) {
  if (platforms.length === 0 && withoutLinks === 0) return null
  return (
    <div className="career-enter career-delay-1 mb-5">
      <div className="mb-2 text-xs font-semibold uppercase tracking-wide text-gray-400">Links by platform</div>
      <div className="flex gap-3 overflow-x-auto pb-1">
        {platforms.map((p) => {
          const accent = accentFor(p.platform)
          return (
            <div
              key={p.platform}
              className="flex min-w-36 shrink-0 items-center gap-3 rounded-2xl border bg-white px-4 py-3 shadow-sm"
              style={{ borderColor: accent.soft }}
            >
              <span
                className="inline-flex h-10 w-10 items-center justify-center rounded-xl text-lg font-bold text-white"
                style={{ background: accent.to }}
                aria-hidden="true"
              >
                {initialOf(p.platform)}
              </span>
              <div>
                <div className="text-2xl font-bold leading-none text-gray-900">{p.count}</div>
                <div className="mt-1 max-w-28 truncate text-xs text-gray-500" title={p.platform}>
                  {p.platform}
                </div>
              </div>
            </div>
          )
        })}
        {average !== null && (
          <div className="flex min-w-36 shrink-0 items-center gap-3 rounded-2xl border border-gray-200 bg-white px-4 py-3 shadow-sm">
            <div>
              <div className="text-2xl font-bold leading-none text-gray-900">{average}%</div>
              <div className="mt-1 text-xs text-gray-500">Average completeness</div>
            </div>
          </div>
        )}
        {withoutLinks > 0 && (
          <button
            type="button"
            onClick={onShowWithoutLinks}
            className="flex min-w-44 shrink-0 flex-col items-start justify-center rounded-2xl border-2 border-dashed border-amber-300 bg-amber-50 px-4 py-3 text-left hover:bg-amber-100"
          >
            <span className="text-sm font-semibold text-amber-800">{plural(withoutLinks, 'profile has', 'profiles have')} no links</span>
            <span className="text-xs text-amber-700">Show the first →</span>
          </button>
        )}
      </div>
    </div>
  )
}
