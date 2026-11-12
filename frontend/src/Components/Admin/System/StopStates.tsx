import { accents } from '../../../Shared/Components/CardArt/accents'
import { WidgetIconGlyph } from '../../../Shared/Components/CardArt/icons'
import { Decor } from '../../../Shared/Components/CardArt/shapes'

// The things the list can be instead of cards (plan 259): a few grey skeleton cards while it loads, a friendly "all quiet" when nothing has stopped, and a card
// for an error with the way to try again.
export function StopSkeletons() {
  return (
    <div aria-label="Loading" role="status" className="space-y-3">
      {[0, 1, 2, 3].map((i) => (
        <div
          key={i}
          className="cinqo-stop-in flex animate-pulse items-center gap-4 rounded-2xl border border-gray-100 bg-white p-4 shadow-sm"
          style={{ '--i': i } as React.CSSProperties}
        >
          <div className="h-10 w-10 shrink-0 rounded-2xl bg-gray-100" />
          <div className="w-44 space-y-2">
            <div className="h-3.5 w-36 rounded-full bg-gray-100" />
            <div className="h-3 w-20 rounded-full bg-gray-100" />
          </div>
          <div className="hidden h-6 w-28 rounded-full bg-gray-100 md:block" />
          <div className="hidden h-3.5 flex-1 rounded-full bg-gray-100 md:block" />
          <div className="h-6 w-16 rounded-full bg-gray-100" />
        </div>
      ))}
    </div>
  )
}

export function StopsEmpty() {
  const a = accents.green
  return (
    <div
      data-testid="stops-empty"
      className="cinqo-stop-in relative overflow-hidden rounded-3xl border p-10 text-center shadow-sm"
      style={{ borderColor: a.soft, background: `linear-gradient(160deg, #ffffff 40%, ${a.soft} 100%)` }}
    >
      <Decor shape="circles" accent={a} />
      <div className="relative mx-auto flex max-w-md flex-col items-center gap-3">
        <span
          className="cinqo-bob inline-flex h-16 w-16 items-center justify-center rounded-3xl text-white shadow-md"
          style={{ background: `linear-gradient(135deg, ${a.from}, ${a.to})` }}
        >
          <WidgetIconGlyph name="shield-check" className="h-8 w-8" />
        </span>
        <h2 className="text-lg font-semibold text-gray-900">All quiet</h2>
        <p className="text-sm text-gray-600">No unexpected stops recorded.</p>
        <p className="text-xs text-gray-500">When Cinqo or the local model stops without being asked to, it shows up here.</p>
      </div>
    </div>
  )
}

export function StopsError({ message, onRetry }: { message: string; onRetry: () => void }) {
  const a = accents.rose
  return (
    <div
      role="alert"
      className="cinqo-stop-in relative flex flex-wrap items-center gap-4 overflow-hidden rounded-3xl border p-5 shadow-sm"
      style={{ borderColor: a.from, background: `linear-gradient(160deg, #ffffff 40%, ${a.soft} 100%)` }}
    >
      <Decor shape="ring" accent={a} />
      <span
        className="relative inline-flex h-10 w-10 shrink-0 items-center justify-center rounded-2xl text-white"
        style={{ background: `linear-gradient(135deg, ${a.from}, ${a.to})` }}
      >
        <WidgetIconGlyph name="alert" />
      </span>
      <p className="relative min-w-0 flex-1 text-sm font-medium" style={{ color: a.text }}>
        {message}
      </p>
      <button
        type="button"
        onClick={onRetry}
        className="relative rounded-full bg-white px-4 py-1.5 text-xs font-semibold shadow-sm ring-1 ring-inset transition-colors hover:bg-gray-50"
        style={{ color: a.text, ['--tw-ring-color' as string]: a.from }}
      >
        Retry
      </button>
    </div>
  )
}
