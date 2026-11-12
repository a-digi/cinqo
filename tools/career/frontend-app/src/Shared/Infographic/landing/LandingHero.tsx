import { LANDING } from './landingTokens'

// The hero band of the landing look: a full-width deep-navy band with a very large bold title, a lead sentence, the buttons, and (at the right on a wide
// window) a calm picture. `ghost` is a huge outlined word behind the picture, only decoration.
export function LandingHero({
  titleId,
  title,
  lead,
  actions,
  picture,
  ghost,
}: {
  titleId?: string
  title: string
  lead: string
  actions?: React.ReactNode
  picture?: React.ReactNode
  ghost?: string
}) {
  return (
    <header
      data-testid="landing-hero"
      className="relative -mx-6 -mt-5 overflow-hidden px-6 py-14 text-white md:py-20"
      style={{ background: `linear-gradient(135deg, ${LANDING.navy} 0%, ${LANDING.navySoft} 100%)` }}
    >
      {ghost !== undefined && (
        <span
          data-testid="hero-ghost"
          aria-hidden="true"
          className="landing-ghost pointer-events-none absolute -right-6 top-1/2 hidden -translate-y-1/2 select-none text-[9rem] font-black uppercase leading-none tracking-tighter md:block lg:text-[13rem]"
          style={{ color: 'transparent', WebkitTextStroke: '2px rgba(255,255,255,0.14)' }}
        >
          {ghost}
        </span>
      )}
      <div className="relative mx-auto grid max-w-6xl items-center gap-10 md:grid-cols-[minmax(0,1.1fr)_minmax(0,1fr)]">
        <div className="min-w-0">
          <h2 id={titleId} className="landing-hero-in text-5xl font-black leading-[1.05] tracking-tight md:text-7xl" style={{ '--i': 0 } as React.CSSProperties}>
            {title}
          </h2>
          <p className="landing-hero-in mt-5 max-w-xl text-base leading-relaxed text-white/80 md:text-lg" style={{ '--i': 1 } as React.CSSProperties}>{lead}</p>
          {actions !== undefined && <div className="landing-hero-in mt-7 flex flex-wrap items-center gap-3" style={{ '--i': 2 } as React.CSSProperties}>{actions}</div>}
        </div>
        {picture !== undefined && <div className="landing-hero-in relative min-w-0" style={{ '--i': 3 } as React.CSSProperties}>{picture}</div>}
      </div>
    </header>
  )
}
