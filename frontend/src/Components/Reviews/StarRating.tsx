import { useId, useState } from 'react'
import { fractionAcross, formatRating, minRating, ratingFromPoint, ratingText, starFills, stepRating, type StarFill } from './reviewRules'

export const STAR_COLOUR = '#f59e0b'
const EMPTY_COLOUR = '#d1d5db'

const SIZES = { sm: 14, md: 20, lg: 28 } as const

function StarShape({ px, colour }: { px: number; colour: string }) {
  return (
    <svg width={px} height={px} viewBox="0 0 24 24" fill={colour} aria-hidden="true" focusable="false">
      <path d="M12 2.5l2.94 6.2 6.8.86-5 4.72 1.3 6.72L12 17.6 5.96 21l1.3-6.72-5-4.72 6.8-.86L12 2.5z" />
    </svg>
  )
}

// One star, empty, half or full: a grey star with the coloured one laid over it and cut off at half or full width.
function Star({ fill, px }: { fill: StarFill; px: number }) {
  const width = fill === 'full' ? px : fill === 'half' ? px / 2 : 0
  return (
    <span className="relative inline-block shrink-0" style={{ width: px, height: px }} data-fill={fill}>
      <span className="absolute left-0 top-0">
        <StarShape px={px} colour={EMPTY_COLOUR} />
      </span>
      <span className="absolute left-0 top-0 overflow-hidden" style={{ width }}>
        <StarShape px={px} colour={STAR_COLOUR} />
      </span>
    </span>
  )
}

interface StarRatingProps {
  rating: number
  scale: number
  halfStars: boolean
  size?: keyof typeof SIZES
  // Without onChange the stars are only shown; with it they are a control (hover shows what a click would give, arrows change it).
  onChange?: (rating: number) => void
  disabled?: boolean
  // What the control is for, read by a screen reader ("Your rating").
  label?: string
}

export function StarRating({ rating, scale, halfStars, size = 'md', onChange, disabled = false, label = 'Rating' }: StarRatingProps) {
  const px = SIZES[size]
  const labelId = useId()
  const [hover, setHover] = useState<number | null>(null)

  if (!onChange) {
    return (
      <span role="img" aria-label={ratingText(rating, scale)} className="inline-flex items-center gap-0.5">
        {starFills(rating, scale).map((fill, i) => (
          <Star key={i} fill={fill} px={px} />
        ))}
      </span>
    )
  }

  // What a pointer on the stars would give: which star it is over and how far across it, null when it is not over a star.
  function pointed(e: React.MouseEvent): number | null {
    const star = (e.target as HTMLElement).closest<HTMLElement>('[data-star]')
    if (!star) return null
    const box = star.getBoundingClientRect()
    return ratingFromPoint(Number(star.dataset.star) - 1, fractionAcross(e.clientX, box.left, box.width), halfStars)
  }

  const shown = hover ?? rating
  const interactive = !disabled
  return (
    <span className="inline-flex items-center gap-2">
      <span id={labelId} className="sr-only">
        {label}
      </span>
      <span
        role="slider"
        tabIndex={interactive ? 0 : -1}
        aria-labelledby={labelId}
        aria-disabled={!interactive}
        aria-valuemin={minRating(halfStars)}
        aria-valuemax={scale}
        aria-valuenow={rating}
        aria-valuetext={rating > 0 ? ratingText(rating, scale) : 'No rating yet'}
        onKeyDown={(e) => {
          if (!interactive) return
          const next = stepRating(rating, e.key, scale, halfStars)
          if (next === null) return
          e.preventDefault()
          if (next !== rating) onChange(next)
        }}
        onMouseMove={(e) => {
          if (interactive) setHover(pointed(e))
        }}
        onClick={(e) => {
          if (!interactive) return
          const r = pointed(e)
          if (r !== null) onChange(r)
        }}
        onMouseLeave={() => {
          setHover(null)
        }}
        className={`inline-flex items-center gap-0.5 rounded-md focus:outline-none focus-visible:ring-2 focus-visible:ring-amber-400 ${interactive ? 'cursor-pointer' : 'cursor-default opacity-60'}`}
      >
        {starFills(shown, scale).map((fill, i) => (
          <span key={i} data-star={i + 1}>
            <Star fill={fill} px={px} />
          </span>
        ))}
      </span>
      <span className="min-w-[2.5rem] text-sm tabular-nums text-gray-600" aria-hidden="true">
        {shown > 0 ? `${formatRating(shown)} / ${scale}` : ''}
      </span>
    </span>
  )
}
