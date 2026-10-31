import type { Accent } from './accents'

// Shapes drawn behind a card's content. Different kinds get different ones, so the page is not a wall of identical
// rectangles.
export type DecorShape = 'circles' | 'diamond' | 'ring'

// `light` is for a pale card (the dashboard): soft accent-coloured shapes. `onColor` is for a coloured head (the
// tool cards): translucent white shapes on the gradient.
export type DecorVariant = 'light' | 'onColor'

// Soft shapes tucked behind the content, in the accent colour. Each kind gets its
// own so the cards do not all look alike.
export function Decor({ shape, accent, variant = 'light' }: { shape: DecorShape; accent: Accent; variant?: DecorVariant }) {
  const onColor = variant === 'onColor'
  const fill = (opacity: number) =>
    onColor
      ? { background: '#ffffff', opacity: opacity * 1.1 }
      : { background: `linear-gradient(135deg, ${accent.from}, ${accent.to})`, opacity }
  const stroke = (opacity: number) => ({ borderColor: onColor ? '#ffffff' : accent.from, opacity: onColor ? opacity * 1.3 : opacity })
  if (shape === 'diamond') {
    return (
      <>
        <div className="pointer-events-none absolute -right-8 -top-8 h-28 w-28 rotate-45 rounded-3xl" style={fill(0.12)} />
        <div className="pointer-events-none absolute right-10 top-12 h-8 w-8 rotate-45 rounded-lg" style={fill(0.16)} />
      </>
    )
  }
  if (shape === 'ring') {
    return (
      <>
        <div className="pointer-events-none absolute -right-10 -top-10 h-32 w-32 rounded-full border-[14px]" style={stroke(0.14)} />
        <div className="pointer-events-none absolute -bottom-6 -left-6 h-20 w-20 rounded-full" style={fill(0.1)} />
      </>
    )
  }
  return (
    <>
      <div className="pointer-events-none absolute -right-6 -top-6 h-28 w-28 rounded-full" style={fill(0.14)} />
      <div className="pointer-events-none absolute right-14 top-16 h-10 w-10 rounded-full" style={fill(0.2)} />
    </>
  )
}
