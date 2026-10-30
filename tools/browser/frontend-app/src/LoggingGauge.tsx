import { ChallengeIcon, CodeIcon, ListIcon } from './icons'
import { levelWord, type Layer } from './debugRules'
import { PAGE_ACCENT } from './Shared/theme'

const ICON: Record<Layer['key'], React.ReactNode> = {
  crawl: <ListIcon className="h-5 w-5" />,
  html: <CodeIcon className="h-5 w-5" />,
  challenge: <ChallengeIcon className="h-5 w-5" />,
}

// The Debug page's overview: a column on the left with an icon per thing that can be logged, the
// base "Crawl logs" at the bottom and the most detailed at the top, joined by a line. An icon is
// green when that layer is being logged right now, amber with a dashed ring when its own toggle is
// on but Debug is off, grey otherwise. The level (0 to 3) is how many are lit.
export function LoggingGauge({ layers, level }: { layers: Layer[]; level: number }) {
  // Most detailed on top, the base at the bottom.
  const stacked = [...layers].reverse()
  return (
    <section className="browser-enter browser-delay-1 rounded-3xl border bg-white p-4 shadow-sm" style={{ borderColor: PAGE_ACCENT.soft }}>
      <div className="flex items-baseline justify-between">
        <h2 className="text-sm font-semibold text-gray-800">Logging level</h2>
        <span className={`text-xs font-semibold ${level === 0 ? 'text-gray-500' : 'text-green-700'}`}>
          {level} · {levelWord(level)}
        </span>
      </div>

      <ol className="mt-4" aria-label="What is being logged">
        {stacked.map((layer, i) => (
          <li key={layer.key} className="relative flex items-start gap-3 pb-4 last:pb-0">
            {i < stacked.length - 1 && (
              <span
                aria-hidden="true"
                className="absolute left-[19px] top-10 h-[calc(100%-2.25rem)] w-0.5"
                style={{ background: stacked[i + 1].on ? '#16a34a' : '#e5e7eb' }}
              />
            )}
            <span
              className={`flex h-10 w-10 shrink-0 items-center justify-center rounded-full ${
                layer.on
                  ? 'bg-green-500 text-white shadow-sm'
                  : layer.pending
                    ? 'border-2 border-dashed border-amber-400 bg-amber-50 text-amber-600'
                    : 'bg-gray-100 text-gray-400'
              }`}
            >
              {ICON[layer.key]}
            </span>
            <div className="min-w-0 pt-0.5">
              <div className={`text-sm font-semibold ${layer.on ? 'text-gray-900' : 'text-gray-500'}`}>{layer.label}</div>
              <div className={`text-xs ${layer.pending ? 'text-amber-700' : 'text-gray-500'}`}>
                {layer.pending ? 'Set, but Debug is off' : layer.detail}
              </div>
            </div>
          </li>
        ))}
      </ol>

      <p className="mt-3 border-t pt-3 text-xs text-gray-500" style={{ borderColor: PAGE_ACCENT.soft }}>
        {level === 0 ? 'Nothing is logged: no crawl writes to Crawl Logs.' : 'Crawls are being written to Crawl Logs.'}
      </p>
    </section>
  )
}
