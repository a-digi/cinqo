import type { CareerExperience } from '../../api'
import { PAGE_ACCENT, accentFor } from '../JobsPage/jobsTheme'
import { buildTimeline, formatCovered, yearTicks } from './timelineRules'

const LANE_HEIGHT = 30

// The Experience page's overview: a horizontal career timeline above the entries. The
// axis runs from the earliest start to the latest end (today for a current role); each
// role is a segment in its company's colour, overlapping roles in their own rows, a
// current role open-ended. The corner says how many years and months are covered.
export function CareerTimeline({ entries, today }: { entries: CareerExperience[]; today: Date }) {
  const timeline = buildTimeline(entries, today)
  const byId = new Map(entries.map((e) => [e.id, e]))
  const ticks = yearTicks(timeline.axisStart, timeline.axisEnd)
  const undated = timeline.undated.map((id) => byId.get(id)).filter((e): e is CareerExperience => e !== undefined)

  return (
    <section className="mb-5 rounded-3xl border bg-white p-4 shadow-sm" style={{ borderColor: PAGE_ACCENT.soft }}>
      <div className="mb-3 flex flex-wrap items-baseline justify-between gap-2">
        <h2 className="text-sm font-semibold text-gray-800">Career timeline</h2>
        {timeline.segments.length > 0 && (
          <span className="text-xs text-gray-500">
            {formatCovered(timeline.coveredMonths)} covered · {Math.floor(timeline.axisStart / 12)} to{' '}
            {timeline.segments.some((s) => s.open) ? 'now' : Math.floor((timeline.axisEnd - 1) / 12)}
          </span>
        )}
      </div>

      {timeline.segments.length > 0 && (
        <>
          <div className="relative" style={{ height: timeline.lanes * LANE_HEIGHT }}>
            {timeline.segments.map((s) => {
              const e = byId.get(s.id)
              if (!e) return null
              const accent = accentFor(e.company)
              return (
                <div
                  key={s.id}
                  title={`${e.title} · ${e.company} (${e.startDate} – ${e.endDate || 'present'})`}
                  className="absolute flex items-center overflow-hidden px-2 text-xs font-medium text-white shadow-sm"
                  style={{
                    left: `${s.left}%`,
                    width: `${s.width}%`,
                    top: s.lane * LANE_HEIGHT,
                    height: LANE_HEIGHT - 4,
                    background: s.open ? `linear-gradient(90deg, ${accent.to}, ${accent.from})` : accent.to,
                    borderRadius: s.open ? '9999px 4px 4px 9999px' : 9999,
                    borderRight: s.open ? `3px dashed ${accent.soft}` : undefined,
                  }}
                >
                  <span className="truncate">{e.title}</span>
                </div>
              )
            })}
          </div>
          <div className="relative mt-1 h-5 border-t border-gray-200">
            {ticks.map((t) => (
              <span key={t.label} className="absolute top-0.5 -translate-x-1/2 text-[10px] text-gray-400" style={{ left: `${t.left}%` }}>
                {t.label}
              </span>
            ))}
          </div>
        </>
      )}

      {timeline.segments.length === 0 && undated.length === 0 && (
        <p className="text-xs text-gray-500">Add a role with a start date and it appears here.</p>
      )}
      {undated.length > 0 && (
        <p className="mt-2 text-xs text-gray-500">
          No readable dates for: {undated.map((e) => `${e.title} · ${e.company}`).join(', ')}. Dates look like 2021, 2021-06 or 2021-06-30.
        </p>
      )}
    </section>
  )
}
