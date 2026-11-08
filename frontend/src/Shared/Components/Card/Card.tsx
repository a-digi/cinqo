// A generic elevated card shell — first built for the Tools admin page
// (a real-estate-listing-style layout: an edge-to-edge media banner up
// top, padded content below), kept generic so any future "grid of
// things" page can reuse it with its own media/content. Introduces
// this app's first rounded-2xl + shadow "elevated card" convention —
// no existing card/shadow style was found to match, so this is a
// deliberate visual upgrade, not a guess at an existing one. Purely
// presentational — no API calls, no context reads.
export interface CardProps {
  children: React.ReactNode
  // Rendered edge-to-edge above the padded content area, no padding of
  // its own applied here — mirrors a listing card's own photo area.
  // Omit entirely for a plain padded card with no banner.
  media?: React.ReactNode
  className?: string
  // Slides the card in from the left when it mounts. Off by default.
  animate?: boolean
  // Start delay for `animate`, in ms — lets a grid of cards stagger.
  animationDelayMs?: number
  // Pinned to the very bottom of the card (plan 218): the content area takes all the free height, so in a row of cards every footer (an Install button,
  // say) sits on one line whatever the text above is. Omit it and the card looks as before.
  footer?: React.ReactNode
}

export function Card({ children, media, className = '', animate = false, animationDelayMs = 0, footer }: CardProps) {
  return (
    <div
      className={`flex flex-col overflow-hidden rounded-2xl border border-gray-100 bg-white shadow-sm transition-shadow hover:shadow-md ${animate ? 'cinqo-card-enter' : ''} ${className}`}
      style={animate && animationDelayMs > 0 ? { animationDelay: `${animationDelayMs}ms` } : undefined}
    >
      {media}
      <div className="flex-1 p-4">{children}</div>
      {footer && <div className="px-4 pb-4">{footer}</div>}
    </div>
  )
}
