import { chartGeometry, formatBarValue, seriesColors, showBarValues, type ChartBox } from '../widgetDisplay'
import type { ChartData } from '../widgetKinds'
import type { Accent } from '../widgetTheme'

// Room above the plot for the value labels printed over the tallest bars.
const box: ChartBox = { width: 420, height: 150, padX: 8, padTop: 20, padBottom: 22 }

// The first series is drawn in the widget's own accent colour; further series
// keep distinct colours so they can be told apart.
export function ChartWidget({ data, accent, gradientId }: { data: ChartData; accent: Accent; gradientId: string }) {
  const g = chartGeometry(data, box)
  const solid = (series: number) => (series === 0 ? accent.to : seriesColors[series % seriesColors.length])
  const barValues = showBarValues(g.bars)
  const fillFor = (series: number) => (series === 0 ? `url(#${gradientId}-bar)` : solid(series))
  return (
    <div>
      <svg viewBox={`0 0 ${box.width} ${box.height}`} role="img" aria-label="Chart" className="h-auto w-full">
        <defs>
          <linearGradient id={`${gradientId}-bar`} x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" stopColor={accent.from} />
            <stop offset="100%" stopColor={accent.to} />
          </linearGradient>
          <linearGradient id={`${gradientId}-area`} x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" stopColor={accent.from} stopOpacity="0.35" />
            <stop offset="100%" stopColor={accent.from} stopOpacity="0" />
          </linearGradient>
        </defs>
        <line x1={box.padX} x2={box.width - box.padX} y1={g.baselineY} y2={g.baselineY} stroke="#e5e7eb" strokeDasharray="3 4" />
        {g.bars.map((b, i) => (
          <rect key={i} x={b.x} y={b.y} width={b.w} height={b.h} rx={Math.min(8, b.w / 2)} fill={fillFor(b.series)} />
        ))}
        {barValues &&
          g.bars.map((b, i) => (
            <text
              key={`v${i}`}
              x={b.x + b.w / 2}
              y={b.value >= 0 ? b.y - 5 : b.y + b.h + 11}
              textAnchor="middle"
              fontSize={10}
              fontWeight={600}
              fill="#374151"
            >
              {formatBarValue(b.value)}
            </text>
          ))}
        {g.lines.map((l) => (
          <g key={l.series}>
            {l.series === 0 && <path d={l.area} fill={`url(#${gradientId}-area)`} />}
            <path d={l.d} fill="none" stroke={solid(l.series)} strokeWidth={2.5} strokeLinecap="round" strokeLinejoin="round" />
            {l.dots.length <= 20 &&
              l.dots.map((d, i) => <circle key={i} cx={d.x} cy={d.y} r={3} fill="#ffffff" stroke={solid(l.series)} strokeWidth={2} />)}
          </g>
        ))}
        {g.labels.map((l) => (
          <text key={`${l.x}-${l.text}`} x={l.x} y={box.height - 6} textAnchor="middle" fontSize={10} fill="#6b7280">
            {l.text}
          </text>
        ))}
      </svg>
      {data.series.length > 1 && (
        <ul className="mt-2 flex flex-wrap gap-x-4 gap-y-1 text-xs text-gray-600">
          {data.series.map((s, i) => (
            <li key={s.name} className="flex items-center gap-1.5">
              <span className="inline-block h-2.5 w-2.5 rounded-full" style={{ background: solid(i) }} />
              {s.name}
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}
