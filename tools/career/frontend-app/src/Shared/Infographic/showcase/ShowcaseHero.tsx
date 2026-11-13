import { SHOWCASE } from './showcaseTokens'

// The hero of the showcase look: a black band, a very large tightly set headline, a lead, the buttons and, under them, a picture. Each part rises in after the other
// (showcase-rise, off under reduced motion). `titleId` names the section by its heading.
export function ShowcaseHero({
  titleId,
  title,
  lead,
  actions,
  picture,
}: {
  titleId?: string
  title: string
  lead: string
  actions?: React.ReactNode
  picture?: React.ReactNode
}) {
  return (
    <header
      data-testid="showcase-hero"
      className="relative -mx-6 -mt-5 overflow-hidden px-6 pb-20 pt-24 text-center md:pt-32"
      style={{ background: SHOWCASE.black, color: SHOWCASE.white, fontFamily: SHOWCASE.font }}
    >
      <div
        aria-hidden="true"
        data-testid="hero-glow"
        className="pointer-events-none absolute left-1/2 top-1/3 h-[32rem] w-[60rem] max-w-none -translate-x-1/2 rounded-full opacity-40 blur-3xl"
        style={{ background: `radial-gradient(closest-side, ${SHOWCASE.blue}, transparent)` }}
      />
      <div className="relative mx-auto max-w-5xl">
        <h2
          id={titleId}
          className="showcase-rise text-6xl font-semibold leading-none tracking-[-0.03em] md:text-8xl"
          style={{ '--i': 0 } as React.CSSProperties}
        >
          {title}
        </h2>
        <p
          className="showcase-rise mx-auto mt-6 max-w-2xl text-xl leading-snug md:text-2xl"
          style={{ color: SHOWCASE.mutedOnDark, '--i': 1 } as React.CSSProperties}
        >
          {lead}
        </p>
        {actions !== undefined && (
          <div className="showcase-rise mt-9 flex flex-wrap items-center justify-center gap-4" style={{ '--i': 2 } as React.CSSProperties}>
            {actions}
          </div>
        )}
        {picture !== undefined && (
          <div className="showcase-rise mx-auto mt-16" style={{ '--i': 3 } as React.CSSProperties}>
            {picture}
          </div>
        )}
      </div>
    </header>
  )
}
