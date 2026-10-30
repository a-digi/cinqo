import { ringArcs, type RingSegment } from './ringMath'

const RADIUS = 46
const STROKE = 12
const CIRCUMFERENCE = 2 * Math.PI * RADIUS

export interface RailRow {
  label: string
  value: string
  // The dot in front of the label.
  color: string
  // Draw this row stronger (something needs attention).
  emphasis?: boolean
}

// A flat summary for the side of a list page: a ring with a figure in its middle, and below
// it a list of figures joined to their labels by dotted leader lines, like a table of
// contents. No boxes or cards: it sits directly on the page. See Portals and Crawl Monitor.
export function SummaryRail({
  title,
  segments,
  centerValue,
  centerLabel,
  rows,
  children,
}: {
  title: string
  segments: RingSegment[]
  centerValue: string
  centerLabel: string
  rows: RailRow[]
  // Anything that belongs under the figures (a control, a note).
  children?: React.ReactNode
}) {
  const arcs = ringArcs(segments, CIRCUMFERENCE)
  return (
    <div>
      <p className="mb-3 text-[11px] font-semibold uppercase tracking-wider text-gray-400">{title}</p>

      <div className="relative mx-auto h-36 w-36">
        <svg viewBox="0 0 120 120" className="h-full w-full -rotate-90" aria-hidden="true">
          <circle cx="60" cy="60" r={RADIUS} fill="none" stroke="#eef2f7" strokeWidth={STROKE} />
          {arcs.map((a, i) => (
            <circle
              key={i}
              cx="60"
              cy="60"
              r={RADIUS}
              fill="none"
              stroke={a.color}
              strokeWidth={STROKE}
              strokeDasharray={`${a.dash} ${CIRCUMFERENCE - a.dash}`}
              strokeDashoffset={a.offset}
            />
          ))}
        </svg>
        <div className="absolute inset-0 flex flex-col items-center justify-center">
          <span className="text-xl font-bold leading-none text-gray-900">{centerValue}</span>
          <span className="mt-1 text-[11px] text-gray-500">{centerLabel}</span>
        </div>
      </div>

      <ul className="mt-5 space-y-2.5">
        {rows.map((r) => (
          <li key={r.label} className="flex items-baseline gap-2 text-sm">
            <span className="h-2 w-2 shrink-0 translate-y-[-1px] rounded-full" style={{ background: r.color }} aria-hidden="true" />
            <span className={r.emphasis ? 'font-semibold text-gray-900' : 'text-gray-600'}>{r.label}</span>
            <span className="min-w-3 flex-1 translate-y-[-3px] border-b border-dotted border-gray-300" aria-hidden="true" />
            <span className={`tabular-nums ${r.emphasis ? 'font-bold text-gray-900' : 'font-semibold text-gray-800'}`}>{r.value}</span>
          </li>
        ))}
      </ul>

      {children}
    </div>
  )
}
