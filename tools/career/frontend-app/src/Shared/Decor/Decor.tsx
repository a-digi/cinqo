import type { Accent } from '../../Components/JobsPage/jobsTheme'
import { gradient } from '../../Components/JobsPage/jobsTheme'
import type { DecorShape } from './decorShapes'

// Soft shapes tucked behind a card's content, in the card's accent colour, like the dashboard cards and this
// tool's page headers. The card must be `relative overflow-hidden`; the shapes never catch the pointer. Put the
// content above them with `relative`.
export function Decor({ shape, accent }: { shape: DecorShape; accent: Accent }) {
  const fill = (opacity: number) => ({ background: gradient(accent), opacity })
  const stroke = (opacity: number) => ({ borderColor: accent.from, opacity })
  if (shape === 'diamond') {
    return (
      <>
        <div
          className="pointer-events-none absolute -right-8 -top-8 h-28 w-28 rotate-45 rounded-3xl"
          style={fill(0.12)}
          aria-hidden="true"
        />
        <div className="pointer-events-none absolute right-24 top-12 h-8 w-8 rotate-45 rounded-lg" style={fill(0.18)} aria-hidden="true" />
        <div
          className="pointer-events-none absolute -bottom-6 left-1/3 h-14 w-14 rotate-45 rounded-2xl"
          style={fill(0.08)}
          aria-hidden="true"
        />
      </>
    )
  }
  if (shape === 'ring') {
    return (
      <>
        <div
          className="pointer-events-none absolute -right-10 -top-10 h-32 w-32 rounded-full border-[14px]"
          style={stroke(0.14)}
          aria-hidden="true"
        />
        <div className="pointer-events-none absolute -bottom-6 -left-6 h-20 w-20 rounded-full" style={fill(0.1)} aria-hidden="true" />
        <div className="pointer-events-none absolute bottom-3 right-1/3 h-6 w-6 rounded-full" style={fill(0.16)} aria-hidden="true" />
      </>
    )
  }
  return (
    <>
      <div className="pointer-events-none absolute -right-6 -top-6 h-28 w-28 rounded-full" style={fill(0.14)} aria-hidden="true" />
      <div className="pointer-events-none absolute right-28 top-14 h-10 w-10 rounded-full" style={fill(0.2)} aria-hidden="true" />
      <div className="pointer-events-none absolute -bottom-8 left-1/4 h-16 w-16 rounded-full" style={fill(0.08)} aria-hidden="true" />
    </>
  )
}
