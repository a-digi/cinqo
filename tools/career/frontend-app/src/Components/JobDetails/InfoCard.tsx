import type { ReactNode } from 'react'
import { Decor } from '../../Shared/Decor/Decor'
import { gradient } from '../JobsPage/jobsTheme'
import { CARD_LOOKS, type CardKey } from './cardLooks'

// The frame of a card on a job's page (step 146), in the dashboard cards' style: a pale tint of the card's accent, a soft border and
// shapes behind the content. The children sit above the shapes.
export function Frame({
  look,
  label,
  className = '',
  children,
}: {
  look: CardKey
  label: string
  className?: string
  children: ReactNode
}) {
  const { accent, shape } = CARD_LOOKS[look]
  return (
    <section
      aria-label={label}
      className={`career-enter career-delay-2 relative w-full overflow-hidden rounded-2xl border p-5 shadow-sm ${className}`}
      style={{ borderColor: accent.soft, background: `linear-gradient(160deg, #ffffff 40%, ${accent.soft} 100%)` }}
    >
      <Decor shape={shape} accent={accent} />
      <div className="relative">{children}</div>
    </section>
  )
}

// The round gradient badge with an icon that opens a card's title.
export function IconBadge({ look, children }: { look: CardKey; children: ReactNode }) {
  return (
    <span
      className="inline-flex h-8 w-8 shrink-0 items-center justify-center rounded-full text-white shadow-sm"
      style={{ background: gradient(CARD_LOOKS[look].accent) }}
      aria-hidden="true"
    >
      {children}
    </span>
  )
}

// A card: the frame, the badge and the title in the accent's colour, then the card's own content.
export function InfoCard({
  look,
  icon,
  title,
  hint,
  children,
}: {
  look: CardKey
  icon: ReactNode
  title: string
  hint?: string
  children: ReactNode
}) {
  return (
    <Frame look={look} label={title}>
      <div className="mb-3 flex items-center gap-2.5">
        <IconBadge look={look}>{icon}</IconBadge>
        <h2 className="min-w-0 text-sm font-semibold" style={{ color: CARD_LOOKS[look].accent.text }}>
          {title}
        </h2>
      </div>
      {hint !== undefined && <p className="mb-3 text-xs text-gray-500">{hint}</p>}
      {children}
    </Frame>
  )
}
