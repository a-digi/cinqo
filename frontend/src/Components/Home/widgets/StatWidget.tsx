import { Link } from 'react-router-dom'
import { deltaDisplay, formatStatValue } from '../widgetDisplay'
import { safeLink, type StatData } from '../widgetKinds'
import type { Accent } from '../../../Shared/Components/CardArt/accents'

const toneColors = { good: '#16a34a', bad: '#dc2626', neutral: '#6b7280' } as const
const toneSoft = { good: '#dcfce7', bad: '#fee2e2', neutral: '#f3f4f6' } as const

export function StatWidget({ data, accent }: { data: StatData; accent: Accent }) {
  const link = safeLink(data.link)
  const delta = data.delta ? deltaDisplay(data.delta) : null
  const body = (
    <div className="flex h-full flex-col justify-center">
      <p className="text-sm text-gray-500">{data.label}</p>
      <p className="mt-1 flex items-baseline gap-1.5">
        <span
          className="bg-clip-text text-5xl font-bold tracking-tight text-transparent"
          style={{ backgroundImage: `linear-gradient(135deg, ${accent.from}, ${accent.to})` }}
        >
          {formatStatValue(data.value)}
        </span>
        {data.unit && <span className="text-sm text-gray-500">{data.unit}</span>}
      </p>
      {(delta !== null || Boolean(data.caption)) && (
        <p className="mt-3 flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-gray-500">
          {delta && (
            <span
              className="inline-flex items-center gap-1 rounded-full px-2 py-0.5 font-semibold"
              style={{ background: toneSoft[delta.tone], color: toneColors[delta.tone] }}
            >
              {delta.arrow} {delta.text}
            </span>
          )}
          {data.caption && (
            <span className="rounded-full px-2 py-0.5 font-medium" style={{ background: accent.soft, color: accent.text }}>
              {data.caption}
            </span>
          )}
        </p>
      )}
    </div>
  )
  return link ? (
    <Link to={link} className="block h-full rounded-2xl">
      {body}
    </Link>
  ) : (
    body
  )
}
