import { Reveal } from '../Reveal'
import { LANDING, type LandingAccent } from './landingTokens'

export type TileVariant = 'wide' | 'half' | 'tall'

// A large pale-grey rounded tile (the landing look's building block): a heavy heading, a line or two of text, the actions (black pill buttons) and a
// picture. `wide` puts the text at the left and the picture at the right across the page; `half` stacks them (two of them make a pair, see TilePair);
// `tall` is a taller half tile with a diagonal shape behind its picture.
export function Tile({
  variant = 'wide',
  titleId,
  title,
  text,
  actions,
  picture,
  delay = 0,
  icon,
  accent,
}: {
  variant?: TileVariant
  // The id of the heading, so the section around can be named by it.
  titleId?: string
  title: string
  text?: string
  actions?: React.ReactNode
  picture?: React.ReactNode
  delay?: number
  // A line icon shown in a round dark chip above the heading.
  icon?: React.ReactNode
  // The chapter's colour: tints the tile, colours the icon chip and draws a bar along the top.
  accent?: LandingAccent
}) {
  const wide = variant === 'wide'
  return (
    <Reveal delay={delay} className="h-full">
      <div
        data-testid="tile"
        data-variant={variant}
        className={`relative h-full overflow-hidden rounded-[1.25rem] p-6 transition-transform hover:-translate-y-0.5 motion-reduce:transition-none motion-reduce:hover:translate-y-0 md:p-10 ${
          wide ? 'grid items-center gap-8 md:grid-cols-[minmax(0,1fr)_minmax(0,1.15fr)]' : 'flex flex-col gap-6'
        } ${variant === 'tall' ? 'md:min-h-[26rem]' : ''}`}
        style={
          {
            background: accent ? `linear-gradient(160deg, ${accent.soft} 0%, ${LANDING.tile} 70%)` : LANDING.tile,
            color: LANDING.ink,
            '--accent': accent?.from ?? LANDING.ink,
          } as React.CSSProperties
        }
      >
        {accent && (
          <span
            data-testid="tile-bar"
            aria-hidden="true"
            className="landing-bar absolute inset-x-0 top-0 h-1.5"
            style={{ background: `linear-gradient(90deg, ${accent.from}, ${accent.to})` }}
          />
        )}
        {variant === 'tall' && (
          <span
            data-testid="tile-diagonal"
            aria-hidden="true"
            className="pointer-events-none absolute inset-x-0 bottom-0 h-1/2"
            style={{ background: '#d9dbe1', clipPath: 'polygon(0 100%, 100% 0, 100% 100%)' }}
          />
        )}
        <div className="relative min-w-0">
          {icon !== undefined && (
            <span
              data-testid="tile-icon"
              aria-hidden="true"
              className="landing-chip relative mb-4 inline-flex h-12 w-12 items-center justify-center rounded-2xl text-white"
              style={{
                background: accent ? `linear-gradient(135deg, ${accent.from}, ${accent.to})` : LANDING.ink,
                boxShadow: accent ? `0 8px 18px -6px ${accent.to}` : undefined,
              }}
            >
              {icon}
            </span>
          )}
          <h3
            id={titleId}
            className="landing-rise text-3xl font-extrabold leading-tight tracking-tight md:text-4xl"
            style={{ '--i': 0 } as React.CSSProperties}
          >
            {title}
          </h3>
          {text !== undefined && text !== '' && (
            <p
              className="landing-rise mt-3 text-base leading-relaxed md:text-lg"
              style={{ color: LANDING.muted, '--i': 1 } as React.CSSProperties}
            >
              {text}
            </p>
          )}
          {actions !== undefined && (
            <div className="landing-rise mt-5 flex flex-wrap items-center gap-2" style={{ '--i': 2 } as React.CSSProperties}>
              {actions}
            </div>
          )}
        </div>
        {picture !== undefined && <div className="landing-pic relative min-w-0">{picture}</div>}
      </div>
    </Reveal>
  )
}

// Two tiles side by side on a wide window, one under the other on a narrow one.
export function TilePair({ children }: { children: React.ReactNode }) {
  return <div className="grid gap-4 md:grid-cols-2">{children}</div>
}
