import type { SystemEventsSummary } from '../../../api/system'
import { accents, type Accent } from '../../../Shared/Components/CardArt/accents'
import { WidgetIconGlyph, type WidgetIcon } from '../../../Shared/Components/CardArt/icons'
import { Decor, type DecorShape } from '../../../Shared/Components/CardArt/shapes'
import { CountUp } from '../../../Shared/Components/Motion/CountUp'
import { latestNewText } from './unexpectedStopsRules'

function Tile({
  label,
  icon,
  accent,
  shape,
  glow,
  testId,
  children,
}: {
  label: string
  icon: WidgetIcon
  accent: Accent
  shape: DecorShape
  glow?: boolean
  testId: string
  children: React.ReactNode
}) {
  return (
    <div
      data-testid={testId}
      className={`cinqo-stop-in relative overflow-hidden rounded-2xl border bg-white/80 p-4 backdrop-blur-sm ${glow ? 'cinqo-fx-glow' : ''}`}
      style={{ borderColor: accent.soft }}
    >
      <Decor shape={shape} accent={accent} />
      <div className="relative flex items-center gap-3">
        <span
          className="inline-flex h-9 w-9 shrink-0 items-center justify-center rounded-xl text-white shadow-sm"
          style={{ background: `linear-gradient(135deg, ${accent.from}, ${accent.to})` }}
        >
          <WidgetIconGlyph name={icon} className="h-[1.1rem] w-[1.1rem]" />
        </span>
        <div className="min-w-0">
          <p className="text-[11px] font-semibold uppercase tracking-wider text-gray-400">{label}</p>
          <div className="truncate text-xl font-bold" style={{ color: accent.text }}>
            {children}
          </div>
        </div>
      </div>
    </div>
  )
}

// Three tiles under the title (plan 259): how many stops there are, how many of them are new (it glows while there are some, and says so when there are none),
// and how long ago the newest new one was. The numbers count up when they appear.
export function StopStats({ total, summary }: { total: number; summary: SystemEventsSummary | null }) {
  const unseen = summary?.unseen ?? null
  return (
    <div className="relative mt-6 grid gap-3 sm:grid-cols-3">
      <Tile label="All stops" icon="layers" accent={accents.blue} shape="circles" testId="stat-total">
        <CountUp value={String(total)} />
      </Tile>
      <Tile
        label="New"
        icon={unseen === 0 ? 'shield-check' : 'alert'}
        accent={unseen === 0 ? accents.green : accents.rose}
        shape="ring"
        glow={unseen !== null && unseen > 0}
        testId="stat-new"
      >
        {unseen === null ? (
          <span className="text-gray-300">…</span>
        ) : unseen === 0 ? (
          <span className="text-base">All seen</span>
        ) : (
          <CountUp value={String(unseen)} />
        )}
      </Tile>
      <Tile label="Latest new stop" icon="clock" accent={accents.violet} shape="diamond" testId="stat-latest">
        {summary === null ? <span className="text-gray-300">…</span> : <span className="text-base">{latestNewText(summary.latest)}</span>}
      </Tile>
    </div>
  )
}
