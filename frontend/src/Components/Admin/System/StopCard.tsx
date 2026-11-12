import { Link } from 'react-router-dom'
import type { SystemEventListItem } from '../../../api/system'
import { unexpectedStopPath } from '../../../config/routing/systemPaths'
import { eventTitle } from '../../../Layout/eventsRules'
import { accents } from '../../../Shared/Components/CardArt/accents'
import { IconBadge } from '../../../Shared/Components/CardArt/icons'
import { formatDateTime, kindLook, relativeTime, seenTitle } from './unexpectedStopsRules'

// The columns of the list on a wide window: the kind's icon, when, what, why, the status and the link. The header row and every card share them.
export const STOP_COLUMNS = 'md:grid-cols-[3rem_13rem_11rem_minmax(0,1fr)_6.5rem_6rem]'

// One stop as a card (plan 259): a stop that is not seen yet has a coloured bar at its left edge, a soft tint and a pulsing "New"; one that is seen is quiet.
// The whole card is one link to the page of the stop (the time is the link, stretched over the card); "Details" is the same link for people who look for it.
// The card lifts when it is pointed at, the arrow moves, and the cards come in one after the other (cinqo-stop-in, --i is the place).
export function StopCard({ stop, index }: { stop: SystemEventListItem; index: number }) {
  const look = kindLook(stop.kind)
  const accent = accents[look.accent]
  const unseen = !stop.acknowledged
  const href = unexpectedStopPath(stop.id)
  return (
    <div
      role="row"
      data-testid="stop-row"
      data-new={unseen ? 'true' : 'false'}
      className={`cinqo-stop-in group relative grid items-center gap-x-4 gap-y-1.5 overflow-hidden rounded-2xl border p-4 pl-5 shadow-sm transition-all duration-200 focus-within:ring-2 focus-within:ring-gray-300 hover:-translate-y-0.5 hover:shadow-lg ${STOP_COLUMNS}`}
      style={
        {
          '--i': index,
          borderColor: unseen ? accent.from : '#f3f4f6',
          background: unseen ? `linear-gradient(100deg, ${accent.soft} 0%, #ffffff 60%)` : '#ffffff',
        } as React.CSSProperties
      }
    >
      {unseen && (
        <span
          aria-hidden="true"
          data-testid="stop-bar"
          className="absolute inset-y-0 left-0 w-1.5"
          style={{ background: `linear-gradient(180deg, ${accent.from}, ${accent.to})` }}
        />
      )}
      <span
        aria-hidden="true"
        className="pointer-events-none absolute -right-7 -top-7 h-24 w-24 rounded-full opacity-[0.09] transition-transform duration-500 group-hover:scale-125"
        style={{ background: `linear-gradient(135deg, ${accent.from}, ${accent.to})` }}
      />
      <div role="cell">
        <IconBadge name={look.icon} from={accent.from} to={accent.to} />
      </div>
      <div role="cell" className="min-w-0">
        <Link to={href} className="font-semibold text-gray-900 after:absolute after:inset-0 after:content-[''] focus:outline-none">
          {formatDateTime(stop.at)}
        </Link>
        <p className="text-xs text-gray-500">{relativeTime(stop.at)}</p>
      </div>
      <div role="cell">
        <span
          className="inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium"
          style={{ background: accent.soft, color: accent.text }}
        >
          {eventTitle(stop)}
        </span>
      </div>
      <div role="cell" className="min-w-0">
        <p className="line-clamp-2 text-sm text-gray-600" title={stop.verdict}>
          {stop.verdict}
        </p>
      </div>
      <div role="cell">
        {unseen ? (
          <span className="inline-flex items-center gap-1.5 rounded-full bg-red-100 px-2.5 py-0.5 text-xs font-semibold text-red-700">
            <span aria-hidden="true" className="relative flex h-2 w-2">
              <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-red-400 opacity-75 motion-reduce:animate-none" />
              <span className="relative inline-flex h-2 w-2 rounded-full bg-red-500" />
            </span>
            New
          </span>
        ) : (
          <span
            title={seenTitle(stop) || undefined}
            className="inline-flex items-center gap-1 rounded-full bg-gray-100 px-2.5 py-0.5 text-xs font-medium text-gray-600"
          >
            <svg
              aria-hidden="true"
              viewBox="0 0 20 20"
              className="h-3 w-3"
              fill="none"
              stroke="currentColor"
              strokeWidth="2.2"
              strokeLinecap="round"
              strokeLinejoin="round"
            >
              <path d="m4.5 10.5 3.5 3.5 7.5-8" />
            </svg>
            Seen
          </span>
        )}
      </div>
      <div role="cell" className="md:text-right">
        <Link
          to={href}
          aria-label={`Details of the stop at ${formatDateTime(stop.at)}`}
          className="relative z-10 inline-flex items-center gap-1 text-xs font-semibold"
          style={{ color: accent.text }}
        >
          Details
          <span aria-hidden="true" className="transition-transform duration-200 group-hover:translate-x-1">
            →
          </span>
        </Link>
      </div>
    </div>
  )
}

// The header row above the cards, for a wide window; its labels are the column headers of the list.
export function StopListHeader() {
  return (
    <div
      role="row"
      className={`hidden gap-x-4 px-5 text-[11px] font-semibold uppercase tracking-wider text-gray-400 md:grid ${STOP_COLUMNS}`}
    >
      <span role="columnheader" className="sr-only">
        Kind
      </span>
      <span role="columnheader">When</span>
      <span role="columnheader">What</span>
      <span role="columnheader">Why (the app&apos;s guess)</span>
      <span role="columnheader">Status</span>
      <span role="columnheader" className="sr-only">
        Details
      </span>
    </div>
  )
}
