import type { ReactNode } from 'react'
import type { Accent } from '../CardArt/accents'
import { WidgetIconGlyph, type WidgetIcon } from '../CardArt/icons'
import { Decor, type DecorShape } from '../CardArt/shapes'

// The coloured head of a card (plan/ai/tools/tool-card-heads.md): a gradient, a few translucent shapes drifting
// slowly behind, a dot grid in one corner, and the icon in a frosted squircle next to the title, like the cards
// on the dashboard. Pass it as a Card's `media`, and give that Card the class `group` so the icon lifts on hover.
// The gradient's two ends are chosen by the caller (see headGradient) so white text is readable on both.
// Purely presentational.
export interface CardHeadProps {
  gradient: { from: string; to: string }
  // The accent the shapes are drawn from, and which set of shapes.
  accent: Accent
  shape: DecorShape
  icon: WidgetIcon
  title: string
  // Small things pinned to the top corners (a status pill, action buttons).
  topLeft?: ReactNode
  topRight?: ReactNode
}

export function CardHead({ gradient, accent, shape, icon, title, topLeft, topRight }: CardHeadProps) {
  return (
    <div
      className="relative h-32 overflow-hidden text-white"
      style={{ background: `linear-gradient(135deg, ${gradient.from}, ${gradient.to})` }}
    >
      <div className="cinqo-slide-drift pointer-events-none absolute inset-0" aria-hidden="true">
        <Decor shape={shape} accent={accent} variant="onColor" />
      </div>
      <div
        className="pointer-events-none absolute inset-0"
        aria-hidden="true"
        style={{ background: 'radial-gradient(circle at 88% 0%, rgba(255,255,255,0.22), transparent 55%)' }}
      />
      <div
        className="pointer-events-none absolute inset-0 opacity-60"
        aria-hidden="true"
        style={{
          backgroundImage: 'radial-gradient(rgba(255,255,255,0.4) 1.2px, transparent 1.2px)',
          backgroundSize: '14px 14px',
          maskImage: 'linear-gradient(to bottom left, black, transparent 55%)',
          WebkitMaskImage: 'linear-gradient(to bottom left, black, transparent 55%)',
        }}
      />
      {topLeft && <div className="absolute left-3 top-3">{topLeft}</div>}
      {topRight && <div className="absolute right-3 top-3">{topRight}</div>}
      <div className="absolute inset-x-4 bottom-4 flex items-center gap-3">
        <span
          className="inline-flex h-12 w-12 shrink-0 items-center justify-center rounded-2xl bg-white/20 shadow-sm ring-1 ring-white/40 backdrop-blur-sm transition-transform duration-300 group-hover:scale-105"
          aria-hidden="true"
        >
          <WidgetIconGlyph name={icon} className="h-6 w-6" />
        </span>
        <h3 className="min-w-0 truncate text-lg font-bold" title={title}>
          {title}
        </h3>
      </div>
    </div>
  )
}
