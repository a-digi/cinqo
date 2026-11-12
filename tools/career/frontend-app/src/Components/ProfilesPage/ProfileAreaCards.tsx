import { MenuGlyph } from '../../Shared/Icons/MenuGlyph'
import { CountUp } from '../../Shared/Motion/CountUp'
import { ACCENTS, PAGE_ACCENT } from '../JobsPage/jobsTheme'
import { applicationsPill, type ApplicationNumbersView } from './applicationNumbersRules'
import { isPlainLeftClick, profileAreaLinks } from './profileAreas'

// The row at the bottom of a profile's card (plan 233): one tile per page of the profile (personas, summary, goals, skills, experience,
// education, languages, courses, hobbies), each with the menu's own icon. A tile is a real link: a plain click opens the page inside the tool,
// a middle or modified click leaves it to the browser. The page it opens shows this profile's data only. The last tile, Applications (plan 265), opens the
// profile's job board and shows how many applications it has once that is known (a grey shimmer while it is being read, nothing when it is not known).
export function ProfileAreaCards({
  profileId,
  name,
  applications = { kind: 'unknown' },
}: {
  profileId: string
  name: string
  applications?: ApplicationNumbersView
}) {
  return (
    <nav aria-label={`Pages of ${name}`} className="mt-3 grid grid-cols-2 gap-2 sm:grid-cols-3 lg:grid-cols-5">
      {profileAreaLinks(profileId, name, applications).map((a) => (
        <a
          key={a.key}
          href={a.href}
          title={a.title}
          onClick={(e) => {
            if (!isPlainLeftClick(e)) return
            e.preventDefault()
            window.__cinqoToolBridge.navigate(a.href)
          }}
          className="flex min-w-0 items-center gap-2 rounded-2xl border bg-white px-2.5 py-2 text-xs font-medium text-gray-700 shadow-sm transition-shadow hover:shadow-md focus-visible:outline focus-visible:outline-2 focus-visible:outline-blue-500"
          style={{ borderColor: PAGE_ACCENT.soft }}
        >
          <span
            className="inline-flex h-7 w-7 shrink-0 items-center justify-center rounded-full"
            style={{ background: PAGE_ACCENT.soft, color: PAGE_ACCENT.text }}
          >
            <MenuGlyph svg={a.icon} className="h-4 w-4" />
          </span>
          <span className="min-w-0 truncate">{a.label}</span>
          {a.applications && <ApplicationsPill view={a.applications} />}
        </a>
      ))}
    </nav>
  )
}

// The number of applications on the Applications tile: green with the figure counting up when there are some, grey for none, a shimmer while the numbers are being read, nothing when they are not known.
function ApplicationsPill({ view }: { view: ApplicationNumbersView }) {
  if (view.kind === 'loading') {
    return (
      <span
        data-testid="applications-pill-loading"
        aria-hidden="true"
        className="ml-auto h-5 w-7 shrink-0 animate-pulse rounded-full bg-gray-200 motion-reduce:animate-none"
      />
    )
  }
  const text = applicationsPill(view)
  if (text === null || view.kind !== 'known') return null
  const some = view.numbers.total > 0
  return (
    <span
      data-testid="applications-pill"
      className="ml-auto inline-flex min-w-[1.5rem] shrink-0 items-center justify-center rounded-full px-2 py-0.5 text-xs font-bold tabular-nums"
      style={some ? { background: ACCENTS[1].soft, color: ACCENTS[1].text } : { background: '#f3f4f6', color: '#6b7280' }}
    >
      <CountUp value={text} durationMs={900} />
    </span>
  )
}
