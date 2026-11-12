import { Decor } from '../../Shared/Decor/Decor'
import { CountUp } from '../../Shared/Motion/CountUp'
import { jobBoardPathFor } from '../../paths'
import { columnAccent } from '../JobBoard/jobBoardRules'
import { gradient, PAGE_ACCENT } from '../JobsPage/jobsTheme'
import { applicationsTitle, stripFor, type ApplicationNumbersView, type StripTile } from './applicationNumbersRules'
import { isPlainLeftClick } from './profileAreas'

// The applications of one profile as numbers on its card (plan 265): a big figure (all applications), and next to it how many are in Applied on the board
// (the board's green) and how many wait for a reply (the board's amber, with a soft pulse while there are any). The whole strip is one link to the
// profile's job board. Zero is a quiet tile with a hint; while the numbers are read there are grey placeholders; when they are not known the strip is not drawn.
export function ProfileApplicationNumbers({ profileId, name, view }: { profileId: string; name: string; view: ApplicationNumbersView }) {
  const strip = stripFor(view)
  if (strip.kind === 'hidden') return null
  const href = jobBoardPathFor(profileId)
  if (strip.kind === 'loading') {
    return (
      <div data-testid="applications-strip-loading" aria-hidden="true" className="mt-3 grid grid-cols-2 gap-2 sm:grid-cols-3">
        <span className="col-span-2 h-[4.5rem] animate-pulse rounded-2xl bg-gray-100 motion-reduce:animate-none sm:col-span-1" />
        <span className="h-[4.5rem] animate-pulse rounded-2xl bg-gray-100 motion-reduce:animate-none" />
        <span className="h-[4.5rem] animate-pulse rounded-2xl bg-gray-100 motion-reduce:animate-none" />
      </div>
    )
  }
  const common = 'mt-3 block rounded-2xl focus-visible:outline focus-visible:outline-2 focus-visible:outline-blue-500'
  const open = (e: React.MouseEvent) => {
    if (!isPlainLeftClick(e)) return
    e.preventDefault()
    window.__cinqoToolBridge.navigate(href)
  }
  if (strip.kind === 'empty') {
    return (
      <a
        href={href}
        onClick={open}
        aria-label={applicationsTitle(name, view)}
        data-testid="applications-strip-empty"
        className={`${common} border border-dashed border-gray-300 bg-white/60 px-4 py-3 hover:bg-white`}
      >
        <span className="block text-sm font-semibold text-gray-500">{strip.text}</span>
        <span className="block text-xs text-gray-400">{strip.hint}</span>
      </a>
    )
  }
  return (
    <a href={href} onClick={open} aria-label={applicationsTitle(name, view)} data-testid="applications-strip" className={common}>
      <span className="grid grid-cols-2 gap-2 sm:grid-cols-3">
        {strip.tiles.map((t, i) => (
          <Tile key={t.key} tile={t} index={i} />
        ))}
      </span>
    </a>
  )
}

// The accent of each tile: the page's blue for the whole, the board's own colours for Applied and for the waiting ones (the Stopped column's amber).
const ACCENT = { total: PAGE_ACCENT, applied: columnAccent('applied', ''), waiting: columnAccent('stopped', '') }

function Tile({ tile, index }: { tile: StripTile; index: number }) {
  const accent = ACCENT[tile.key]
  const big = tile.key === 'total'
  return (
    <span
      data-tile={tile.key}
      data-emphasis={tile.emphasis ? 'true' : 'false'}
      className={`relative flex flex-col justify-center overflow-hidden rounded-2xl border px-4 py-3 shadow-sm transition-all hover:-translate-y-0.5 hover:shadow-md motion-reduce:transition-none motion-reduce:hover:translate-y-0 ${big ? 'col-span-2 sm:col-span-1' : ''}`}
      style={{ borderColor: accent.soft, background: `linear-gradient(160deg, #ffffff 40%, ${accent.soft} 100%)` }}
    >
      {big && <Decor shape="ring" accent={accent} />}
      <span className="relative flex items-center gap-2">
        <span
          data-testid={`figure-${tile.key}`}
          className={`font-extrabold leading-none tabular-nums ${big ? 'bg-clip-text text-4xl text-transparent' : 'text-2xl'}`}
          style={big ? { backgroundImage: gradient(accent) } : { color: accent.text }}
        >
          <CountUp value={String(tile.value)} durationMs={1100} delayMs={index * 120} />
        </span>
        {tile.emphasis && (
          <span
            data-testid="waiting-dot"
            aria-hidden="true"
            className="career-wait-pulse h-2.5 w-2.5 rounded-full"
            style={{ background: accent.to }}
          />
        )}
      </span>
      <span className="relative mt-1 text-xs font-medium text-gray-600">{tile.label}</span>
    </span>
  )
}
